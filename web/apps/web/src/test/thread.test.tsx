import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fileRef, type ReplyView, type Spot } from "@cadenza/core";
import ThreadPage, { sortQuestions } from "../pages/Thread.tsx";
import { makeServices, renderWith, threadView, type Routes } from "./helpers.tsx";

const viewer = vi.hoisted(() => ({
  pick: null as unknown as { spot: unknown },
  setMarkers: [] as unknown[],
  renders: 0,
  destroyed: 0,
}));

vi.mock("@cadenza/viewer", () => ({
  renderPost: (el: HTMLElement, attachments: unknown[], opts: { resolveFile: unknown }) => {
    viewer.renders++;
    el.textContent = `viewer with ${attachments.length} attachment(s)`;
    expect(typeof opts.resolveFile).toBe("function");
    return {
      element: el,
      pickSpot: async () => viewer.pick.spot,
      cancelPick() {},
      setMarkers: (m: unknown) => viewer.setMarkers.push(m),
      show() {},
      destroy: () => viewer.destroyed++,
    };
  },
}));

beforeEach(() => {
  viewer.pick = { spot: null };
  viewer.setMarkers = [];
  viewer.renders = 0;
  viewer.destroyed = 0;
});

const ok = () => ({ body: {} });

function setup(view = threadView("t1"), routes: Routes = {}) {
  let current = view;
  const made = makeServices({
    "GET /v1/threads/t1": () => ({ body: current }),
    "GET /v1/threads": () => ({ body: { threads: [current.thread] } }),
    ...routes,
  });
  renderWith(<ThreadPage id="t1" />, made.services);
  return { ...made, update: (v: typeof view) => (current = v) };
}

const question = (extra = {}) => ({
  id: "q_1",
  post_id: "p_1",
  text_html: "<p>Keep the promo field visible?</p>",
  choices: ["Yes", "No"],
  waiting: true,
  status: "open",
  created_at: Date.now() - 1000,
  ...extra,
});

describe("thread view", () => {
  it("shows posts with the viewer, summary html and thread info", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: "Checkout redesign" })).toBeTruthy();
    expect(screen.getByText("viewer with 1 attachment(s)")).toBeTruthy();
    expect(screen.getByText("below").tagName).toBe("STRONG");
    expect(screen.getByText(/Round 1/)).toBeTruthy();
    expect(screen.getByText("Claude Code · vm-03")).toBeTruthy();
    expect(viewer.renders).toBe(1);
  });

  it("answers a question with a choice and a note", async () => {
    const { f } = setup(threadView("t1", { questions: [question()] }), { "POST /v1/questions/q_1/answer": () => ({ body: { reply: "r_1" } }) });
    const q = await screen.findByRole("region", { name: "Question from the agent" });
    expect(within(q).getByText("Agent is waiting")).toBeTruthy();
    expect(within(q).getByText("Keep the promo field visible?")).toBeTruthy();
    const send = within(q).getByRole("button", { name: "Send answer" }) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    fireEvent.click(within(q).getByRole("button", { name: "Yes" }));
    expect(within(q).getByRole("button", { name: "Yes" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.change(within(q).getByLabelText("Add a note (optional)"), { target: { value: "  but smaller  " } });
    fireEvent.click(send);
    await waitFor(() => expect(f.of("POST", "/v1/questions/q_1/answer")).toHaveLength(1));
    expect(f.of("POST", "/v1/questions/q_1/answer")[0]!.body).toEqual({ choice: "Yes", text: "but smaller" });
    expect(f.of("POST", "/v1/uploads")).toHaveLength(0);
  });

  it("answers with a photo: converts to WebP, uploads new files to signed links, then sends their refs", async () => {
    const photo = new File(["jpeg-bytes"], "IMG_0001.JPG", { type: "image/jpeg" });
    const full = new Blob([new Uint8Array(await photo.arrayBuffer()), "-full"], { type: "image/webp" });
    const preview = new Blob(["preview-of-", new Uint8Array(await photo.arrayBuffer())], { type: "image/webp" });
    const fullRef = await fileRef(full, "IMG_0001.webp", "image/webp");
    const previewRef = await fileRef(preview, "IMG_0001.preview.webp", "image/webp");
    const puts: { url: string; headers: unknown }[] = [];
    const { f } = setup(threadView("t1", { questions: [question({ choices: undefined })] }), {
      "POST /v1/uploads": () => ({
        body: { uploads: [{ sha256: fullRef.sha256, url: "https://store.example/put/full", method: "PUT", headers: { "Content-Type": "image/webp" }, expires: 1 }], existing: [previewRef.sha256] },
      }),
      "PUT https://store.example/put/full": (c) => {
        puts.push({ url: c.url, headers: c.headers });
        return ok();
      },
      "POST /v1/questions/q_1/answer": () => ({ body: { reply: "r_1" } }),
    });
    const q = await screen.findByRole("region", { name: "Question from the agent" });
    const input = q.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, { target: { files: [photo] } });
    expect(within(q).getByText("IMG_0001.JPG")).toBeTruthy();
    fireEvent.click(within(q).getByRole("button", { name: "Send answer" }));
    await waitFor(() => expect(f.of("POST", "/v1/questions/q_1/answer")).toHaveLength(1));

    const up = f.of("POST", "/v1/uploads")[0]!.body as { files: { sha256: string; type: string; name: string }[] };
    expect(up.files).toEqual([
      { ...previewRef, name: "IMG_0001.preview.webp" },
      { ...fullRef, preview_sha256: previewRef.sha256 },
    ]);
    expect(puts).toEqual([{ url: "https://store.example/put/full", headers: { "Content-Type": "image/webp" } }]);
    expect(f.of("POST", "/v1/questions/q_1/answer")[0]!.body).toEqual({ files: [{ ...fullRef, preview_sha256: previewRef.sha256 }] });
  });

  it("dismisses a question", async () => {
    const { f } = setup(threadView("t1", { questions: [question()] }), { "POST /v1/questions/q_1/dismiss": ok });
    fireEvent.click(await screen.findByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(f.of("POST", "/v1/questions/q_1/dismiss")).toHaveLength(1));
  });

  it("sends a note and a verdict on the post", async () => {
    const { f } = setup(threadView("t1"), { "POST /v1/replies": () => ({ status: 201, body: { reply: "r_9" } }) });
    const panel = await screen.findByRole("region", { name: "Reply" });
    fireEvent.change(within(panel).getByLabelText("Note"), { target: { value: "Looks good on my phone" } });
    fireEvent.click(within(panel).getByRole("button", { name: "Send note" }));
    await waitFor(() => expect(f.of("POST", "/v1/replies")).toHaveLength(1));
    expect(f.of("POST", "/v1/replies")[0]!.body).toEqual({ post_id: "p_1", kind: "note", text: "Looks good on my phone" });
    expect(await within(panel).findByText("Note sent.")).toBeTruthy();

    fireEvent.click(within(panel).getByRole("button", { name: "Request changes" }));
    await waitFor(() => expect(f.of("POST", "/v1/replies")).toHaveLength(2));
    expect(f.of("POST", "/v1/replies")[1]!.body).toEqual({ post_id: "p_1", kind: "verdict", verdict: "request_changes" });

    fireEvent.click(within(panel).getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(f.of("POST", "/v1/replies")).toHaveLength(3));
    expect(f.of("POST", "/v1/replies")[2]!.body).toEqual({ post_id: "p_1", kind: "verdict", verdict: "approve" });
  });

  it("pins a comment to a spot picked in the viewer", async () => {
    const spot: Spot = { attachment: 0, block: "summary", quote: "Promo field" };
    viewer.pick = { spot };
    const { f } = setup(threadView("t1"), { "POST /v1/replies": () => ({ status: 201, body: { reply: "r_9" } }) });
    const panel = await screen.findByRole("region", { name: "Reply" });
    fireEvent.click(within(panel).getByRole("button", { name: "Pin to a spot" }));
    expect(await within(panel).findByText(/Pinned to “Promo field” in Summary/)).toBeTruthy();
    const send = within(panel).getByRole("button", { name: "Send comment" }) as HTMLButtonElement;
    expect(send.disabled).toBe(true); // a comment needs text
    fireEvent.change(within(panel).getByLabelText("Your comment"), { target: { value: "Make this bold" } });
    fireEvent.click(send);
    await waitFor(() => expect(f.of("POST", "/v1/replies")).toHaveLength(1));
    expect(f.of("POST", "/v1/replies")[0]!.body).toEqual({ post_id: "p_1", kind: "comment", text: "Make this bold", spot });
  });

  it("keeps the note when picking is cancelled", async () => {
    viewer.pick = { spot: null };
    setup();
    const panel = await screen.findByRole("region", { name: "Reply" });
    fireEvent.click(within(panel).getByRole("button", { name: "Pin to a spot" }));
    await waitFor(() => expect(within(panel).getByRole("button", { name: "Pin to a spot" })).toBeTruthy());
    expect(within(panel).queryByText(/Pinned to/)).toBeNull();
    expect(within(panel).getByRole("button", { name: "Send note" })).toBeTruthy();
  });

  it("shows replies with pins, the agent's answer and delivery", async () => {
    const replies: ReplyView[] = [
      { id: "r_1", kind: "comment", thread: "t1", post: "p_1", text: "Bigger button", spot: { attachment: 0, block: "summary", quote: "x" }, created_at: Date.now() - 5000, delivered: true, agent_reply: "Fixed in round 2", agent_reply_at: Date.now() - 1000 },
      { id: "r_2", kind: "note", thread: "t1", post: "p_1", text: "Also check dark mode", created_at: Date.now() - 4000, delivered: false },
      { id: "r_3", kind: "answer", thread: "t1", post: "p_1", question: "q_1", choice: "Yes", created_at: Date.now() - 3000, delivered: true },
    ];
    setup(threadView("t1", { replies, questions: [question({ status: "answered" })] }));
    const box = await screen.findByLabelText("Your replies on this post");
    expect(within(box).getByText("Bigger button")).toBeTruthy();
    expect(within(box).getByText("Fixed in round 2")).toBeTruthy();
    expect(within(box).getByText(/Agent has it/)).toBeTruthy();
    expect(within(box).getByText(/Not picked up yet/)).toBeTruthy();
    expect(within(box).getByLabelText("Pin 1")).toBeTruthy();
    // The answer is shown with its question, not under the post.
    expect(within(box).queryByText("Yes")).toBeNull();
    expect(viewer.setMarkers.at(-1)).toEqual([{ id: "r_1", spot: replies[0]!.spot, label: "1" }]);
    fireEvent.click(screen.getByText("Earlier questions (1)"));
    expect(screen.getByText("Answered")).toBeTruthy();
  });

  it("archives and reopens the thread", async () => {
    const { f, update } = setup(threadView("t1"), { "POST /v1/threads/t1/status": ok });
    fireEvent.click(await screen.findByRole("button", { name: "Archive" }));
    await waitFor(() => expect(f.of("POST", "/v1/threads/t1/status")).toHaveLength(1));
    expect(f.of("POST", "/v1/threads/t1/status")[0]!.body).toEqual({ status: "archived" });
    void update;
  });

  it("shows server errors with their fix", async () => {
    setup(threadView("t1", { questions: [question()] }), {
      "POST /v1/questions/q_1/answer": () => ({ status: 409, body: { error: { kind: "conflict", message: "This question was already answered.", fix: "Reload the thread." } } }),
    });
    const q = await screen.findByRole("region", { name: "Question from the agent" });
    fireEvent.click(within(q).getByRole("button", { name: "No" }));
    fireEvent.click(within(q).getByRole("button", { name: "Send answer" }));
    expect(await within(q).findByText("This question was already answered.")).toBeTruthy();
    expect(within(q).getByText("Reload the thread.")).toBeTruthy();
  });

  it("shows an error when the thread can't be loaded", async () => {
    setup(threadView("t1"), { "GET /v1/threads/t1": () => ({ status: 404, body: { error: { kind: "not_found", message: "No such thread." } } }) });
    expect(await screen.findByText("No such thread.")).toBeTruthy();
  });
});

describe("sortQuestions", () => {
  it("puts open questions first, waiting ones at the top", () => {
    const qs = [
      { ...question({ id: "a", waiting: false, created_at: 1 }) },
      { ...question({ id: "b", waiting: true, created_at: 2 }) },
      { ...question({ id: "c", status: "answered", created_at: 3 }) },
      { ...question({ id: "d", status: "dismissed", created_at: 4 }) },
    ];
    const { open, closed } = sortQuestions(qs);
    expect(open.map((q) => q.id)).toEqual(["b", "a"]);
    expect(closed.map((q) => q.id)).toEqual(["d", "c"]);
  });
});
