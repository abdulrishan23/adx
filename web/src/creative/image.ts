// Browser-only picture helpers: read a file's size, and draw the exact-size
// JPGs the board and the ad platforms get.

export async function readImage(file: Blob) {
  const bitmap = await createImageBitmap(file);
  return { bitmap, width: bitmap.width, height: bitmap.height };
}

// Draws the picture at exactly width × height, cropping to cover if the shape
// differs slightly, on white so transparent PNGs don't turn black.
export async function toJpeg(
  bitmap: ImageBitmap,
  width: number,
  height: number,
  quality: number,
): Promise<Blob> {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("This browser can't draw pictures");
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, width, height);
  const scale = Math.max(width / bitmap.width, height / bitmap.height);
  const dw = bitmap.width * scale;
  const dh = bitmap.height * scale;
  ctx.drawImage(bitmap, (width - dw) / 2, (height - dh) / 2, dw, dh);
  return new Promise((resolve, reject) =>
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("Couldn't encode the picture"))),
      "image/jpeg",
      quality,
    ),
  );
}

// The small preview the lists and the board show: 640 px wide.
export async function previewOf(full: Blob, width = 640) {
  const bmp = await createImageBitmap(full);
  return toJpeg(bmp, width, Math.round(width * (bmp.height / bmp.width)), 0.82);
}
