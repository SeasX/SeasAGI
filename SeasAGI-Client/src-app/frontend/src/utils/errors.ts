export function getErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  if (typeof error === "string" && error.trim()) {
    return error;
  }
  if (error && typeof error === "object") {
    const record = error as Record<string, unknown>;
    const message = [record.message, record.error].find((value) => typeof value === "string" && value.trim());
    if (typeof message === "string") {
      return message;
    }
  }
  return fallback;
}
