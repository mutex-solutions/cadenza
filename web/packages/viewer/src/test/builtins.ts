// Copied from internal/template/builtin/*.json: each built-in template's slots and example.
// Regenerate by hand when the built-ins change.
export interface BuiltinTemplate {
  name: string;
  slots: { name: string; block: string; label: string }[];
  example: Record<string, unknown>;
}

export const BUILTINS: BuiltinTemplate[] = [
  {
    "name": "bug-fix",
    "slots": [
      {
        "name": "symptom",
        "block": "text",
        "label": "Symptom"
      },
      {
        "name": "cause",
        "block": "text",
        "label": "Cause"
      },
      {
        "name": "fix",
        "block": "diff",
        "label": "Fix"
      },
      {
        "name": "proof",
        "block": "code",
        "label": "Proof"
      },
      {
        "name": "screenshot",
        "block": "image",
        "label": "Screenshot"
      },
      {
        "name": "risks",
        "block": "list",
        "label": "Risks"
      }
    ],
    "example": {
      "symptom": "Saving a profile with an emoji in the name showed *\"Something went wrong\"*. Reproduce: set name to `Ana 🌸`, press Save.",
      "cause": "`validateName` in `profile/name.go` counted bytes, not characters, so a 4-byte emoji pushed short names over the 32 limit.",
      "fix": {
        "filename": "profile/name.go",
        "diff": "--- a/profile/name.go\n+++ b/profile/name.go\n@@ -12,3 +12,3 @@ func validateName(s string) error {\n-\tif len(s) > 32 {\n+\tif utf8.RuneCountInString(s) > 32 {\n \t\treturn errTooLong\n \t}\n"
      },
      "proof": {
        "language": "text",
        "code": "=== RUN   TestValidateNameEmoji\n--- PASS: TestValidateNameEmoji (0.00s)\nok  \texample.com/app/profile\t0.012s"
      },
      "risks": [
        "Names stored before the fix are unchanged"
      ]
    }
  },
  {
    "name": "code-change",
    "slots": [
      {
        "name": "summary",
        "block": "text",
        "label": "Summary"
      },
      {
        "name": "files",
        "block": "list",
        "label": "Files changed"
      },
      {
        "name": "diff",
        "block": "diff",
        "label": "Diff"
      },
      {
        "name": "notes",
        "block": "text",
        "label": "Notes for you"
      }
    ],
    "example": {
      "summary": "Moved retry logic out of each HTTP call into one `retry.Do` helper, so all calls back off the same way.",
      "files": [
        "`internal/retry/retry.go`: new helper with jittered backoff",
        "`internal/api/client.go`: three calls now use it"
      ],
      "diff": {
        "filename": "internal/api/client.go",
        "diff": "--- a/internal/api/client.go\n+++ b/internal/api/client.go\n@@ -40,7 +40,3 @@\n-\tfor i := 0; i < 3; i++ {\n-\t\tresp, err = c.http.Do(req)\n-\t\tif err == nil { break }\n-\t\ttime.Sleep(time.Second)\n-\t}\n+\tresp, err := retry.Do(ctx, func() (*http.Response, error) { return c.http.Do(req) })\n"
      },
      "notes": "Backoff now caps at 30 s. Is that too long for the upload path?"
    }
  },
  {
    "name": "data-report",
    "slots": [
      {
        "name": "headline",
        "block": "metrics",
        "label": "Headline numbers"
      },
      {
        "name": "chart",
        "block": "chart",
        "label": "Chart"
      },
      {
        "name": "table",
        "block": "table",
        "label": "Table"
      },
      {
        "name": "findings",
        "block": "list",
        "label": "Findings"
      },
      {
        "name": "method",
        "block": "text",
        "label": "Method"
      }
    ],
    "example": {
      "headline": [
        {
          "label": "Weekly active users",
          "value": 12840,
          "before": 11920,
          "good": "up"
        },
        {
          "label": "Churn",
          "value": "3.1%",
          "before": "3.8%",
          "good": "down"
        }
      ],
      "chart": {
        "mark": "line",
        "data": {
          "values": [
            {
              "week": "W1",
              "users": 11920
            },
            {
              "week": "W2",
              "users": 12210
            },
            {
              "week": "W3",
              "users": 12840
            }
          ]
        },
        "encoding": {
          "x": {
            "field": "week",
            "type": "ordinal"
          },
          "y": {
            "field": "users",
            "type": "quantitative"
          }
        }
      },
      "table": {
        "columns": [
          "Plan",
          "Users",
          "Churn %"
        ],
        "rows": [
          [
            "Free",
            9100,
            4.2
          ],
          [
            "Pro",
            3740,
            0.9
          ]
        ]
      },
      "findings": [
        "Growth comes from Pro: free users are flat",
        "Churn dropped after the onboarding email went out on W2"
      ],
      "method": "Events table, last 3 full weeks, excluding staff and test accounts."
    }
  },
  {
    "name": "design-review",
    "slots": [
      {
        "name": "mockups",
        "block": "gallery",
        "label": "Mockups"
      },
      {
        "name": "reasoning",
        "block": "text",
        "label": "Reasoning"
      },
      {
        "name": "questions",
        "block": "list",
        "label": "Questions"
      }
    ],
    "example": {
      "mockups": [
        {
          "file": "./mock/inbox-cards.png",
          "caption": "A: cards"
        },
        {
          "file": "./mock/inbox-list.png",
          "caption": "B: dense list"
        }
      ],
      "reasoning": "**A** shows a preview image per post, good for UI work. **B** fits twice as many threads on a phone.",
      "questions": [
        "Which fits how you use the inbox?",
        "Is the unread dot visible enough in B?"
      ]
    }
  },
  {
    "name": "experiment",
    "slots": [
      {
        "name": "hypothesis",
        "block": "text",
        "label": "Hypothesis"
      },
      {
        "name": "setup",
        "block": "text",
        "label": "Setup"
      },
      {
        "name": "results",
        "block": "chart",
        "label": "Results"
      },
      {
        "name": "numbers",
        "block": "metrics",
        "label": "Numbers"
      },
      {
        "name": "conclusion",
        "block": "text",
        "label": "Conclusion"
      },
      {
        "name": "next_step",
        "block": "text",
        "label": "Next step"
      }
    ],
    "example": {
      "hypothesis": "Caching parsed templates will cut render time by half, since parsing is 60% of the profile.",
      "setup": "Added an LRU cache (256 entries). Same machine, same 1,000 sample pages, 5 runs each.",
      "results": {
        "mark": "bar",
        "data": {
          "values": [
            {
              "variant": "no cache",
              "ms": 42
            },
            {
              "variant": "cache",
              "ms": 19
            }
          ]
        },
        "encoding": {
          "x": {
            "field": "variant",
            "type": "nominal"
          },
          "y": {
            "field": "ms",
            "type": "quantitative"
          }
        }
      },
      "numbers": [
        {
          "label": "Median render",
          "before": 42,
          "value": 19,
          "unit": "ms",
          "good": "down"
        },
        {
          "label": "Memory",
          "before": "110 MB",
          "value": "126 MB",
          "good": "down"
        }
      ],
      "conclusion": "Right: render time dropped 55% for a 15% memory increase.",
      "next_step": "Ship behind a flag and watch memory on the largest tenant."
    }
  },
  {
    "name": "feature-demo",
    "slots": [
      {
        "name": "summary",
        "block": "text",
        "label": "What it does"
      },
      {
        "name": "walkthrough",
        "block": "video",
        "label": "Walkthrough"
      },
      {
        "name": "screenshots",
        "block": "gallery",
        "label": "Screenshots"
      },
      {
        "name": "try_it",
        "block": "text",
        "label": "How to try it"
      },
      {
        "name": "limits",
        "block": "list",
        "label": "Limits"
      }
    ],
    "example": {
      "summary": "You can now export any report as CSV from the **Share** menu.",
      "walkthrough": {
        "file": "./demo/export.mp4"
      },
      "screenshots": [
        {
          "file": "./demo/share-menu.png",
          "caption": "New Export CSV item"
        },
        {
          "file": "./demo/done.png",
          "caption": "Download finished toast"
        }
      ],
      "try_it": "Open any report, click **Share → Export CSV**.",
      "limits": [
        "Exports at most 50,000 rows",
        "No Excel format yet"
      ]
    }
  },
  {
    "name": "incident",
    "slots": [
      {
        "name": "what_happened",
        "block": "text",
        "label": "What happened"
      },
      {
        "name": "status",
        "block": "callout",
        "label": "Status"
      },
      {
        "name": "timeline",
        "block": "table",
        "label": "Timeline"
      },
      {
        "name": "cause",
        "block": "text",
        "label": "Cause"
      },
      {
        "name": "fix",
        "block": "text",
        "label": "Fix"
      },
      {
        "name": "follow_ups",
        "block": "checklist",
        "label": "Follow-ups"
      }
    ],
    "example": {
      "what_happened": "Uploads failed for all users for 22 minutes.",
      "status": {
        "tone": "success",
        "text": "Resolved at 14:32 UTC."
      },
      "timeline": {
        "columns": [
          "Time (UTC)",
          "Event"
        ],
        "rows": [
          [
            "14:10",
            "Storage credentials rotated"
          ],
          [
            "14:12",
            "Upload errors start"
          ],
          [
            "14:30",
            "Old key restored"
          ],
          [
            "14:32",
            "Uploads recover"
          ]
        ]
      },
      "cause": "The rotation script updated the secret but the server caches credentials until restart.",
      "fix": "Restored the old key, then restarted the server with the new one.",
      "follow_ups": [
        {
          "text": "Reload credentials without restart",
          "done": false
        },
        {
          "text": "Alert on upload error rate above 5%",
          "done": false
        }
      ]
    }
  },
  {
    "name": "options",
    "slots": [
      {
        "name": "question",
        "block": "text",
        "label": "Question"
      },
      {
        "name": "options",
        "block": "choices",
        "label": "Options"
      },
      {
        "name": "recommendation",
        "block": "callout",
        "label": "Recommendation"
      },
      {
        "name": "context",
        "block": "text",
        "label": "Context"
      }
    ],
    "example": {
      "question": "Which database should the trial mode use?",
      "options": [
        {
          "title": "SQLite",
          "text": "One file next to the binary.",
          "pros": [
            "Zero setup",
            "Fast for one user"
          ],
          "cons": [
            "Needs a migration path to Postgres"
          ]
        },
        {
          "title": "Embedded Postgres",
          "text": "Download and run Postgres on first start.",
          "pros": [
            "Same database as production"
          ],
          "cons": [
            "150 MB download",
            "Slow first start"
          ]
        }
      ],
      "recommendation": {
        "tone": "info",
        "text": "**SQLite**: the trial should start in under a minute."
      },
      "context": "Trial mode is the `cadenza serve --trial` path: one user, local only, started from the README."
    }
  },
  {
    "name": "perf-compare",
    "slots": [
      {
        "name": "numbers",
        "block": "metrics",
        "label": "Key numbers"
      },
      {
        "name": "chart",
        "block": "chart",
        "label": "Chart"
      },
      {
        "name": "method",
        "block": "text",
        "label": "How it was measured"
      },
      {
        "name": "verdict",
        "block": "callout",
        "label": "Verdict"
      }
    ],
    "example": {
      "numbers": [
        {
          "label": "p95 latency",
          "before": 340,
          "value": 120,
          "unit": "ms",
          "good": "down"
        },
        {
          "label": "Bundle size",
          "before": "812 KB",
          "value": "640 KB",
          "good": "down"
        }
      ],
      "chart": {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "mark": "bar",
        "data": {
          "values": [
            {
              "run": "before",
              "ms": 340
            },
            {
              "run": "after",
              "ms": 120
            }
          ]
        },
        "encoding": {
          "x": {
            "field": "run",
            "type": "nominal",
            "sort": [
              "before",
              "after"
            ]
          },
          "y": {
            "field": "ms",
            "type": "quantitative",
            "title": "p95 ms"
          }
        }
      },
      "method": "10 runs of `k6 run search.js` (50 virtual users, 60 s) on a 4-core VM against a copy of production data. Median of p95 shown.",
      "verdict": {
        "tone": "success",
        "text": "Search is almost 3× faster at p95. Worth shipping."
      }
    }
  },
  {
    "name": "plan",
    "slots": [
      {
        "name": "goal",
        "block": "text",
        "label": "Goal"
      },
      {
        "name": "steps",
        "block": "checklist",
        "label": "Steps"
      },
      {
        "name": "diagram",
        "block": "diagram",
        "label": "Diagram"
      },
      {
        "name": "risks",
        "block": "list",
        "label": "Risks"
      },
      {
        "name": "questions",
        "block": "list",
        "label": "Open questions"
      }
    ],
    "example": {
      "goal": "Users can sign in with GitHub as well as email.",
      "steps": [
        {
          "text": "Add OAuth app settings",
          "done": true
        },
        {
          "text": "Callback handler and account linking",
          "done": false
        },
        {
          "text": "Sign-in button on web and Android",
          "done": false
        }
      ],
      "diagram": {
        "mermaid": "sequenceDiagram\n  User->>App: Sign in with GitHub\n  App->>GitHub: Redirect\n  GitHub->>App: Callback with code\n  App->>User: Session"
      },
      "risks": [
        "Users with the same email on both methods need linking rules"
      ],
      "questions": [
        "Should GitHub sign-in be allowed on self-hosted servers by default?"
      ]
    }
  },
  {
    "name": "progress",
    "slots": [
      {
        "name": "summary",
        "block": "text",
        "label": "Summary"
      },
      {
        "name": "done",
        "block": "list",
        "label": "Done"
      },
      {
        "name": "doing",
        "block": "list",
        "label": "Doing"
      },
      {
        "name": "next",
        "block": "list",
        "label": "Next"
      },
      {
        "name": "blockers",
        "block": "callout",
        "label": "Blockers"
      }
    ],
    "example": {
      "summary": "Migration is 60% done; on track for Friday.",
      "done": [
        "Users and teams tables migrated",
        "Backfill job tested on staging"
      ],
      "doing": [
        "Migrating the events table (40M rows)"
      ],
      "next": [
        "Switch reads to the new schema",
        "Remove old columns"
      ],
      "blockers": {
        "tone": "warning",
        "text": "Need a 30-minute maintenance window for the final switch."
      }
    }
  },
  {
    "name": "release-notes",
    "slots": [
      {
        "name": "version",
        "block": "text",
        "label": "Version"
      },
      {
        "name": "highlights",
        "block": "list",
        "label": "Highlights"
      },
      {
        "name": "changes",
        "block": "list",
        "label": "Changes"
      },
      {
        "name": "breaking",
        "block": "callout",
        "label": "Breaking changes"
      }
    ],
    "example": {
      "version": "v1.4.0 (2026-11-11)",
      "highlights": [
        "Uploads resume after a lost connection",
        "Dark mode on Android"
      ],
      "changes": [
        "Faster inbox loading",
        "Fixed: emoji in names could not be saved"
      ],
      "breaking": {
        "tone": "warning",
        "text": "`cadenza show --file` is now `cadenza show <path>`. Update scripts."
      }
    }
  },
  {
    "name": "research",
    "slots": [
      {
        "name": "question",
        "block": "text",
        "label": "Question"
      },
      {
        "name": "findings",
        "block": "list",
        "label": "Findings"
      },
      {
        "name": "sources",
        "block": "links",
        "label": "Sources"
      },
      {
        "name": "recommendation",
        "block": "callout",
        "label": "Recommendation"
      }
    ],
    "example": {
      "question": "Can MCP tool calls wait longer than 60 seconds?",
      "findings": [
        "Codex and Cursor cancel tool calls after about 60 s",
        "The MCP spec allows progress notifications, but not all clients reset their timeout on them"
      ],
      "sources": [
        {
          "title": "Model Context Protocol specification",
          "url": "https://modelcontextprotocol.io/specification"
        }
      ],
      "recommendation": {
        "tone": "info",
        "text": "Cap waits at 45 s and let the agent call again."
      }
    }
  },
  {
    "name": "test-report",
    "slots": [
      {
        "name": "verdict",
        "block": "callout",
        "label": "Verdict"
      },
      {
        "name": "counts",
        "block": "metrics",
        "label": "Counts"
      },
      {
        "name": "failures",
        "block": "table",
        "label": "Failures"
      },
      {
        "name": "output",
        "block": "code",
        "label": "Output"
      },
      {
        "name": "coverage",
        "block": "metrics",
        "label": "Coverage"
      },
      {
        "name": "screenshots",
        "block": "gallery",
        "label": "Screenshots"
      }
    ],
    "example": {
      "verdict": {
        "tone": "danger",
        "text": "2 of 214 tests failed, both in checkout."
      },
      "counts": [
        {
          "label": "Passed",
          "value": 210
        },
        {
          "label": "Failed",
          "value": 2,
          "good": "down"
        },
        {
          "label": "Skipped",
          "value": 2
        },
        {
          "label": "Duration",
          "value": "48 s"
        }
      ],
      "failures": {
        "columns": [
          "Test",
          "Reason"
        ],
        "rows": [
          [
            "TestCheckoutPromo",
            "expected total 90, got 100"
          ],
          [
            "TestCheckoutEmptyCart",
            "timeout after 5s"
          ]
        ]
      },
      "output": {
        "language": "text",
        "code": "--- FAIL: TestCheckoutPromo (0.02s)\n    checkout_test.go:88: expected total 90, got 100"
      },
      "coverage": [
        {
          "label": "Lines",
          "value": 81.4,
          "before": 79.9,
          "unit": "%",
          "good": "up"
        }
      ]
    }
  },
  {
    "name": "ui-change",
    "slots": [
      {
        "name": "summary",
        "block": "text",
        "label": "Summary"
      },
      {
        "name": "before",
        "block": "image",
        "label": "Before"
      },
      {
        "name": "after",
        "block": "image",
        "label": "After"
      },
      {
        "name": "demo",
        "block": "video",
        "label": "Demo"
      },
      {
        "name": "changes",
        "block": "list",
        "label": "Changes"
      },
      {
        "name": "risks",
        "block": "list",
        "label": "Risks"
      }
    ],
    "example": {
      "summary": "Promo code field now sits below the order total, so checkout is one step instead of two.",
      "before": {
        "file": "./shots/checkout-before.png"
      },
      "after": {
        "file": "./shots/checkout-after.png"
      },
      "demo": {
        "file": "./shots/checkout.mp4"
      },
      "changes": [
        "Promo field moved under the total",
        "Step 2 (\"Apply code\") removed",
        "Error text now shows inline"
      ],
      "risks": [
        "On screens under 360 px the total wraps to two lines"
      ]
    }
  }
];
