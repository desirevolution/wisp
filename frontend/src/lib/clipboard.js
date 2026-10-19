// Construct ClipboardItem while the click still carries user activation. Safari
// also needs the asynchronous fetch/decode work inside the item's Blob promise.
export function copyImage(message) {
  if (!navigator.clipboard?.write || typeof ClipboardItem === 'undefined') throw new Error('Image copying is unavailable. You can download the image instead.');
  const png = fetch(`/api/media/${encodeURIComponent(message.media_id)}`).then(async response => {
    if (!response.ok) throw new Error('Could not load the image.');
    const blob = await response.blob();
    if (blob.type === 'image/png') return blob;
    const url = URL.createObjectURL(blob), image = new Image();
    try {
      image.src = url; await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.naturalWidth; canvas.height = image.naturalHeight;
      const context = canvas.getContext('2d');
      if (!context) throw new Error('Could not prepare the image.');
      context.drawImage(image, 0, 0);
      return await new Promise((resolve, reject) => canvas.toBlob(value => value ? resolve(value) : reject(new Error('Could not prepare the image.')), 'image/png'));
    } finally { URL.revokeObjectURL(url); }
  });
  // A permission rejection may finish before the image request fails.
  void png.catch(() => {});
  return navigator.clipboard.write([new ClipboardItem({'image/png': png})]);
}

export function copyText(text) {
  if (!navigator.clipboard?.writeText) throw new Error('Text copying is unavailable in this browser or connection.');
  return navigator.clipboard.writeText(text);
}
