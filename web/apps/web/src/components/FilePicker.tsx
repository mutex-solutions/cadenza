import { useId, useRef } from "react";

/** Attach photos (or other files). On phones the picker offers the camera too. */
export function FilePicker({ files, onChange, label = "Add photo" }: { files: File[]; onChange: (f: File[]) => void; label?: string }) {
  const input = useRef<HTMLInputElement>(null);
  const id = useId();
  return (
    <div>
      <input
        ref={input}
        id={id}
        type="file"
        accept="image/*,application/pdf,video/mp4"
        multiple
        hidden
        aria-label={label}
        onChange={(e) => {
          const picked = Array.from(e.target.files ?? []);
          if (picked.length) onChange([...files, ...picked].slice(0, 10));
          e.target.value = "";
        }}
      />
      <button type="button" onClick={() => input.current?.click()}>
        {label}
      </button>
      {files.length > 0 && (
        <div className="thumbs" aria-label="Attached files">
          {files.map((f, i) => (
            <span key={`${f.name}-${i}`}>
              {f.name}{" "}
              <button type="button" className="ghost" style={{ minHeight: 20, padding: "0 4px" }} aria-label={`Remove ${f.name}`} onClick={() => onChange(files.filter((_, j) => j !== i))}>
                ×
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
