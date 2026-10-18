// Only native camera captures are optimized; ordinary file/paste uploads stay intact.
export async function prepareCameraImage(file) {
  if (!file || !['image/jpeg','image/png'].includes(file.type) || !globalThis.createImageBitmap) return file;
  let bitmap;
  try {
    bitmap = await createImageBitmap(file);
    const scale = Math.min(1, 2048 / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.size <= 1024 * 1024) return file;
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(bitmap.width * scale));
    canvas.height = Math.max(1, Math.round(bitmap.height * scale));
    const context = canvas.getContext('2d');
    if (!context) return file;
    context.fillStyle = '#fff';context.fillRect(0,0,canvas.width,canvas.height);
    context.drawImage(bitmap,0,0,canvas.width,canvas.height);
    const blob = await new Promise(resolve => canvas.toBlob(resolve,'image/jpeg',0.85));
    if (!blob?.size || blob.size >= file.size) return file;
    return new File([blob], `${file.name.replace(/\.[^.]*$/, '') || 'camera'}.jpg`, {type:'image/jpeg',lastModified:file.lastModified});
  } catch {
    // If this browser cannot decode/re-encode the photo, retain normal validation.
    return file;
  } finally { bitmap?.close(); }
}
