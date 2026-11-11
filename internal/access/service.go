package access

import (
	"context"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Service implements access's use cases.
type Service struct {
	store Store
	clock kernel.Clock
	pub   Publisher
}

func NewService(store Store, clock kernel.Clock, pub Publisher) *Service {
	return &Service{store: store, clock: clock, pub: pub}
}

// HasOwner reports whether first-run setup is done.
func (s *Service) HasOwner(ctx context.Context) (bool, error) {
	_, err := s.store.GetOwner(ctx)
	if kernel.Is(err, kernel.KindNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Setup creates the owner with a password. It fails once an owner exists, so
// it is only usable on a fresh server.
func (s *Service) Setup(ctx context.Context, password string) error {
	if err := CheckPasswordRules(password); err != nil {
		return err
	}
	hash := HashPassword(password)
	return s.store.InTx(ctx, func(r Repo) error {
		if _, err := r.GetOwner(ctx); err == nil {
			return kernel.Conflict("the owner password is already set").WithFix("log in, or reset it with `cadenza-server owner reset-password`")
		} else if !kernel.Is(err, kernel.KindNotFound) {
			return err
		}
		return r.SaveOwner(ctx, &Owner{ID: kernel.OwnerID(kernel.NewID("o")), PasswordHash: hash, CreatedAt: s.clock.Now()})
	})
}

// ResetPassword replaces the owner password, or creates the owner. It is only
// reachable from the server's own command line.
func (s *Service) ResetPassword(ctx context.Context, password string) error {
	if err := CheckPasswordRules(password); err != nil {
		return err
	}
	hash := HashPassword(password)
	return s.store.InTx(ctx, func(r Repo) error {
		o, err := r.GetOwner(ctx)
		if kernel.Is(err, kernel.KindNotFound) {
			return r.SaveOwner(ctx, &Owner{ID: kernel.OwnerID(kernel.NewID("o")), PasswordHash: hash, CreatedAt: s.clock.Now()})
		}
		if err != nil {
			return err
		}
		o.PasswordHash = hash
		return r.SaveOwner(ctx, o)
	})
}

// Login checks the password and returns a new web session token.
func (s *Service) Login(ctx context.Context, password string) (string, error) {
	o, err := s.store.GetOwner(ctx)
	if kernel.Is(err, kernel.KindNotFound) {
		return "", kernel.Conflict("no owner password is set yet").WithFix("open the app to set it up")
	}
	if err != nil {
		return "", err
	}
	if !VerifyPassword(password, o.PasswordHash) {
		return "", kernel.Unauthorized("wrong password")
	}
	token := kernel.NewSecret(PrefixWebSession)
	now := s.clock.Now()
	err = s.store.CreateWebSession(ctx, &WebSession{TokenHash: kernel.HashSecret(token), Owner: o.ID, CreatedAt: now, ExpiresAt: now.Add(WebSessionTTL)})
	return token, err
}

// Logout ends a web session.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.DeleteWebSession(ctx, kernel.HashSecret(token))
}

// Authenticate resolves any Cadenza credential to a Principal.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	switch {
	case strings.HasPrefix(token, PrefixWebSession):
		return s.authWeb(ctx, token)
	case strings.HasPrefix(token, PrefixMachine):
		return s.authMachine(ctx, token)
	case strings.HasPrefix(token, PrefixSession):
		return s.authSession(ctx, token)
	}
	return nil, kernel.Unauthorized("not a Cadenza key")
}

func (s *Service) authWeb(ctx context.Context, token string) (*Principal, error) {
	ws, err := s.store.GetWebSession(ctx, kernel.HashSecret(token))
	if kernel.Is(err, kernel.KindNotFound) {
		return nil, kernel.Unauthorized("logged out").WithFix("log in again")
	}
	if err != nil {
		return nil, err
	}
	if !s.clock.Now().Before(ws.ExpiresAt) {
		return nil, kernel.Unauthorized("your login expired").WithFix("log in again")
	}
	return &Principal{Role: RoleOwner, Owner: ws.Owner}, nil
}

func (s *Service) authMachine(ctx context.Context, token string) (*Principal, error) {
	m, err := s.store.GetMachineByKeyHash(ctx, kernel.HashSecret(token))
	if kernel.Is(err, kernel.KindNotFound) {
		return nil, kernel.Unauthorized("this machine is not paired").WithFix("run `cadenza login <server-url>`")
	}
	if err != nil {
		return nil, err
	}
	if m.Removed() {
		return nil, kernel.Unauthorized("this machine was removed").WithFix("run `cadenza login <server-url>` to pair it again")
	}
	if err := s.touchMachine(ctx, m); err != nil {
		return nil, err
	}
	return &Principal{Role: RoleMachine, Owner: m.Owner, MachineID: m.ID, MachineName: m.Name}, nil
}

func (s *Service) authSession(ctx context.Context, token string) (*Principal, error) {
	as, err := s.store.GetAgentSessionByKeyHash(ctx, kernel.HashSecret(token))
	if kernel.Is(err, kernel.KindNotFound) {
		return nil, kernel.Unauthorized("unknown session key")
	}
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if !as.Usable(now) {
		return nil, kernel.Unauthorized("this session key expired or was revoked").WithFix("run the command again; cadenza gets a new key automatically")
	}
	m, err := s.store.GetMachine(ctx, as.Owner, as.MachineID)
	if err != nil {
		return nil, err
	}
	if m.Removed() {
		return nil, kernel.Unauthorized("this machine was removed").WithFix("run `cadenza login <server-url>` to pair it again")
	}
	if now.Sub(as.LastUsedAt) >= touchEvery {
		as.LastUsedAt = now
		if err := s.store.UpdateAgentSession(ctx, as); err != nil {
			return nil, err
		}
	}
	if err := s.touchMachine(ctx, m); err != nil {
		return nil, err
	}
	return &Principal{Role: RoleSession, Owner: as.Owner, MachineID: m.ID, MachineName: m.Name, SessionID: as.ID, Agent: as.Agent}, nil
}

func (s *Service) touchMachine(ctx context.Context, m *Machine) error {
	now := s.clock.Now()
	if now.Sub(m.LastSeenAt) < touchEvery {
		return nil
	}
	m.LastSeenAt = now
	return s.store.UpdateMachine(ctx, m)
}

// PairStart begins pairing. The code is shown to the person at the machine;
// the secret stays with the CLI and proves it started this pairing.
func (s *Service) PairStart(ctx context.Context, hostname string) (spec.PairStartOutput, error) {
	hostname = clip(strings.TrimSpace(hostname), 64)
	secret := kernel.NewSecret("")
	now := s.clock.Now()
	p := &Pairing{
		ID: kernel.NewID("pr"), Code: NewPairingCode(), SecretHash: kernel.HashSecret(secret),
		Hostname: hostname, Status: PairingPending, CreatedAt: now, ExpiresAt: now.Add(PairingTTL),
	}
	if err := s.store.CreatePairing(ctx, p); err != nil {
		return spec.PairStartOutput{}, err
	}
	return spec.PairStartOutput{PairID: p.ID, Code: p.Code, Secret: secret, Expires: kernel.Millis(p.ExpiresAt)}, nil
}

// PairApprove is the owner accepting a code typed into the app.
func (s *Service) PairApprove(ctx context.Context, owner kernel.OwnerID, code, name string) (spec.PairApproveOutput, error) {
	code = NormalizeCode(code)
	var out spec.PairApproveOutput
	var events []eventlog.Event
	err := s.store.InTx(ctx, func(r Repo) error {
		p, err := r.GetPendingPairingByCode(ctx, code)
		if kernel.Is(err, kernel.KindNotFound) {
			return kernel.NotFound("no pending pairing with code %s", code).WithFix("check the code shown by `cadenza login`; codes last 10 minutes")
		}
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if p.Expired(now) {
			return kernel.Conflict("code %s expired", code).WithFix("run `cadenza login` again for a new code")
		}
		name = clip(strings.TrimSpace(name), 64)
		if name == "" {
			name = p.Hostname
		}
		if name == "" {
			name = "machine"
		}
		m := &Machine{ID: kernel.NewID("m"), Owner: owner, Name: name, CreatedAt: now, LastSeenAt: now,
			// Placeholder hash until the machine claims its key; no key matches it.
			KeyHash: "unclaimed:" + p.ID}
		if err := r.CreateMachine(ctx, m); err != nil {
			return err
		}
		p.Status, p.Owner, p.MachineID, p.MachineName = PairingApproved, owner, m.ID, m.Name
		if err := r.UpdatePairing(ctx, p); err != nil {
			return err
		}
		e := eventlog.Event{Owner: owner, Kind: spec.EventMachineChanged, RefID: m.ID, At: now}
		if err := r.AppendEvent(ctx, &e); err != nil {
			return err
		}
		events = append(events, e)
		out = spec.PairApproveOutput{MachineID: m.ID, Name: m.Name}
		return nil
	})
	if err == nil {
		s.pub.Publish(events...)
	}
	return out, err
}

// PairPoll is the CLI checking on its pairing. The machine key is returned on
// the first poll after approval and never again.
func (s *Service) PairPoll(ctx context.Context, pairID, secret string) (spec.PairPollOutput, error) {
	var out spec.PairPollOutput
	err := s.store.InTx(ctx, func(r Repo) error {
		p, err := r.GetPairing(ctx, pairID)
		if err != nil {
			return err
		}
		if !kernel.SecretEqual(p.SecretHash, kernel.HashSecret(secret)) {
			return kernel.Forbidden("wrong pairing secret")
		}
		now := s.clock.Now()
		switch {
		case p.Status == PairingClaimed:
			return kernel.Conflict("this pairing was already used").WithFix("run `cadenza login` again")
		case p.Status == PairingPending && p.Expired(now):
			out.Status = spec.PairExpired
			return nil
		case p.Status == PairingPending:
			out.Status = spec.PairPending
			return nil
		}
		m, err := r.GetMachine(ctx, p.Owner, p.MachineID)
		if err != nil {
			return err
		}
		key := kernel.NewSecret(PrefixMachine)
		m.KeyHash = kernel.HashSecret(key)
		if err := r.UpdateMachine(ctx, m); err != nil {
			return err
		}
		p.Status = PairingClaimed
		if err := r.UpdatePairing(ctx, p); err != nil {
			return err
		}
		out = spec.PairPollOutput{Status: spec.PairApproved, MachineID: m.ID, MachineName: m.Name, MachineKey: key}
		return nil
	})
	return out, err
}

// MintSessionKey gives an agent session a key. The same agent session id
// always maps to the same session, so resuming an agent keeps its threads; a
// new key replaces the old one.
func (s *Service) MintSessionKey(ctx context.Context, machine *Principal, in spec.SessionKeyInput) (spec.SessionKeyOutput, error) {
	if machine.Role != RoleMachine {
		return spec.SessionKeyOutput{}, kernel.Forbidden("only a machine key can create session keys")
	}
	agent := strings.ToLower(clip(strings.TrimSpace(in.Agent), 32))
	ext := clip(strings.TrimSpace(in.AgentSessionID), 128)
	if agent == "" || ext == "" {
		return spec.SessionKeyOutput{}, kernel.Invalid("agent and agent_session_id are required").
			WithFix("pass --session <id>, or run cadenza inside an agent that sets its session id")
	}
	key := kernel.NewSecret(PrefixSession)
	now := s.clock.Now()
	var out spec.SessionKeyOutput
	err := s.store.InTx(ctx, func(r Repo) error {
		as, err := r.GetAgentSessionByExternal(ctx, machine.MachineID, agent, ext)
		switch {
		case kernel.Is(err, kernel.KindNotFound):
			as = &AgentSession{ID: kernel.NewID("s"), Owner: machine.Owner, MachineID: machine.MachineID, Agent: agent,
				ExternalID: ext, Cwd: clip(in.Cwd, 512), Repo: clip(in.Repo, 512), KeyHash: kernel.HashSecret(key),
				CreatedAt: now, LastUsedAt: now}
			if err := r.CreateAgentSession(ctx, as); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			as.KeyHash, as.LastUsedAt, as.RevokedAt = kernel.HashSecret(key), now, nil
			if in.Cwd != "" {
				as.Cwd = clip(in.Cwd, 512)
			}
			if in.Repo != "" {
				as.Repo = clip(in.Repo, 512)
			}
			if err := r.UpdateAgentSession(ctx, as); err != nil {
				return err
			}
		}
		out = spec.SessionKeyOutput{SessionID: as.ID, SessionKey: key}
		return nil
	})
	return out, err
}

// Machines lists the owner's machines that are not removed.
func (s *Service) Machines(ctx context.Context, owner kernel.OwnerID) (spec.MachineList, error) {
	rows, err := s.store.ListMachines(ctx, owner)
	if err != nil {
		return spec.MachineList{}, err
	}
	out := spec.MachineList{Machines: []spec.Machine{}}
	for _, m := range rows {
		out.Machines = append(out.Machines, spec.Machine{ID: m.ID, Name: m.Name, CreatedAt: kernel.Millis(m.CreatedAt),
			LastSeenAt: kernel.Millis(m.LastSeenAt), Sessions: m.Sessions})
	}
	return out, nil
}

// RenameMachine changes a machine's display name.
func (s *Service) RenameMachine(ctx context.Context, owner kernel.OwnerID, id, name string) error {
	name = clip(strings.TrimSpace(name), 64)
	if name == "" {
		return kernel.Invalid("the name is empty")
	}
	return s.changeMachine(ctx, owner, id, func(m *Machine) { m.Name = name })
}

// RemoveMachine stops the machine's keys at once. What it showed stays.
func (s *Service) RemoveMachine(ctx context.Context, owner kernel.OwnerID, id string) error {
	now := s.clock.Now()
	return s.changeMachine(ctx, owner, id, func(m *Machine) { m.RemovedAt = &now })
}

func (s *Service) changeMachine(ctx context.Context, owner kernel.OwnerID, id string, change func(*Machine)) error {
	var events []eventlog.Event
	err := s.store.InTx(ctx, func(r Repo) error {
		m, err := r.GetMachine(ctx, owner, id)
		if err != nil {
			return err
		}
		if m.Removed() {
			return kernel.NotFound("machine %s was removed", id)
		}
		change(m)
		if err := r.UpdateMachine(ctx, m); err != nil {
			return err
		}
		e := eventlog.Event{Owner: owner, Kind: spec.EventMachineChanged, RefID: m.ID, At: s.clock.Now()}
		if err := r.AppendEvent(ctx, &e); err != nil {
			return err
		}
		events = append(events, e)
		return nil
	})
	if err == nil {
		s.pub.Publish(events...)
	}
	return err
}

// RevokeSession stops one agent session's key.
func (s *Service) RevokeSession(ctx context.Context, owner kernel.OwnerID, id string) error {
	return s.store.InTx(ctx, func(r Repo) error {
		as, err := r.GetAgentSession(ctx, owner, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		as.RevokedAt = &now
		return r.UpdateAgentSession(ctx, as)
	})
}

// Session returns an agent session (for display and checks).
func (s *Service) Session(ctx context.Context, owner kernel.OwnerID, id string) (*AgentSession, error) {
	return s.store.GetAgentSession(ctx, owner, id)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary.
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
