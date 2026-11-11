// Draws the app icon (a ring and a dot on a dark square) as PNGs, with no
// dependencies. Run once: node scripts/make-icons.mjs. Output is committed.
import { writeFileSync } from "node:fs";
import { deflateSync } from "node:zlib";

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});
const crc32 = (buf) => {
  let c = 0xffffffff;
  for (const b of buf) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
};
const chunk = (type, data) => {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const td = Buffer.concat([Buffer.from(type), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(td));
  return Buffer.concat([len, td, crc]);
};

export function drawIcon(size, { rounded = true } = {}) {
  const bg = [0x0d, 0x11, 0x17];
  const fg = [0x4a, 0xa3, 0xff];
  const k = size / 512;
  const c = size / 2;
  const rows = [];
  // 4x4 supersampling for smooth edges.
  for (let y = 0; y < size; y++) {
    const row = Buffer.alloc(1 + size * 4);
    for (let x = 0; x < size; x++) {
      let inSq = 0, inFg = 0;
      for (let sy = 0; sy < 4; sy++)
        for (let sx = 0; sx < 4; sx++) {
          const px = x + (sx + 0.5) / 4, py = y + (sy + 0.5) / 4;
          const r = 112 * k;
          const dx = Math.max(r - px, 0, px - (size - r)), dy = Math.max(r - py, 0, py - (size - r));
          const square = !rounded || dx * dx + dy * dy <= r * r;
          if (!square) continue;
          inSq++;
          const d = Math.hypot(px - c, py - c);
          if (Math.abs(d - 138 * k) <= 22 * k || d <= 34 * k) inFg++;
        }
      const a = inSq / 16, f = inSq ? inFg / inSq : 0;
      const o = 1 + x * 4;
      for (let i = 0; i < 3; i++) row[o + i] = Math.round(bg[i] * (1 - f) + fg[i] * f);
      row[o + 3] = Math.round(a * 255);
    }
    rows.push(row);
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // RGBA
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", ihdr),
    chunk("IDAT", deflateSync(Buffer.concat(rows))),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const out = new URL("../public/", import.meta.url);
  writeFileSync(new URL("icon-192.png", out), drawIcon(192));
  writeFileSync(new URL("icon-512.png", out), drawIcon(512, { rounded: false }));
  writeFileSync(new URL("apple-touch-icon.png", out), drawIcon(180, { rounded: false }));
  console.log("icons written");
}
