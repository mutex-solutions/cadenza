import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import Sketch from "../components/Sketch.tsx";

describe("sketch", () => {
  it("draws, undoes, and sends a picture with its labels", () => {
    const onDone = vi.fn();
    const onCancel = vi.fn();
    const toBlob = vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation(function (cb: BlobCallback) {
      cb(new Blob(["png"], { type: "image/png" }));
    });
    render(<Sketch onDone={onDone} onCancel={onCancel} />);
    const send = screen.getByRole("button", { name: "Send sketch" }) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    const canvas = screen.getByLabelText("Drawing area");
    fireEvent.pointerDown(canvas, { clientX: 5, clientY: 5, pointerId: 1 });
    expect(send.disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(send.disabled).toBe(true);
    fireEvent.pointerDown(canvas, { clientX: 5, clientY: 5, pointerId: 1 });
    fireEvent.change(screen.getByLabelText("Words in your sketch"), { target: { value: "button, total" } });
    fireEvent.click(send);
    expect(onDone).toHaveBeenCalledWith(expect.any(Blob), ["button", "total"]);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onCancel).toHaveBeenCalled();
    toBlob.mockRestore();
  });
});
