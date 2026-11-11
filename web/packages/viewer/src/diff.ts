/** One line of a unified diff, ready to draw. */
export interface DiffRow {
  kind: "file" | "hunk" | "add" | "del" | "ctx" | "meta";
  text: string;
  /** Line number in the old file (del, ctx). */
  old?: number;
  /** Line number in the new file (add, ctx). */
  new?: number;
}

const HUNK = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

/**
 * Parses unified diff text. Lines inside a hunk are counted against the hunk
 * header, so a removed line that starts with "--" is not taken for a file header.
 */
export function parseDiff(text: string): DiffRow[] {
  const lines = text.split("\n");
  if (lines.length > 1 && lines[lines.length - 1] === "") lines.pop();
  const rows: DiffRow[] = [];
  let oldN = 0;
  let newN = 0;
  let oldLeft = 0;
  let newLeft = 0;
  for (const raw of lines) {
    const line = raw.replace(/\r$/, "");
    const inHunk = oldLeft > 0 || newLeft > 0;
    const m = HUNK.exec(line);
    if (m && !inHunk) {
      oldN = Number(m[1]);
      newN = Number(m[3]);
      oldLeft = m[2] === undefined ? 1 : Number(m[2]);
      newLeft = m[4] === undefined ? 1 : Number(m[4]);
      rows.push({ kind: "hunk", text: line });
      continue;
    }
    if (!inHunk) {
      rows.push({ kind: line.startsWith("---") || line.startsWith("+++") ? "file" : "meta", text: line });
      continue;
    }
    const sign = line[0];
    const body = line.slice(1);
    if (sign === "+") {
      rows.push({ kind: "add", text: body, new: newN++ });
      newLeft--;
    } else if (sign === "-") {
      rows.push({ kind: "del", text: body, old: oldN++ });
      oldLeft--;
    } else if (sign === "\\") {
      rows.push({ kind: "meta", text: line });
    } else {
      rows.push({ kind: "ctx", text: sign === " " ? body : line, old: oldN++, new: newN++ });
      oldLeft--;
      newLeft--;
    }
  }
  return rows;
}
