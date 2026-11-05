export type ResponseError = {
  error?: string;
  message?: string;
  provider_type?: string;
};

export async function readResponseError(
  response: Response,
): Promise<ResponseError> {
  const body = await response.text();
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    parsed = undefined;
  }
  if (parsed && typeof parsed === "object") return parsed as ResponseError;
  return {
    message: body.trim() || response.statusText || `HTTP ${response.status}`,
  };
}
