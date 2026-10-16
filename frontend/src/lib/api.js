export class ApiError extends Error {
  constructor(message, status, body = null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

export async function apiJson(url, options) {
  const response = await fetch(url, options);
  const body = await response.json().catch(() => null);
  if (!response.ok) {
    throw new ApiError(body?.error || `Request failed (${response.status})`, response.status, body);
  }
  return body;
}

export async function apiRequest(url, options) {
  const response = await fetch(url, options);
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new ApiError(body?.error || `Request failed (${response.status})`, response.status, body);
  }
  return response;
}
