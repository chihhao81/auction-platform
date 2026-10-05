const backendOrigin = process.env.NEXT_PUBLIC_API_ORIGIN ?? "http://localhost:8080";

export function apiURL(path: string): string {
  return new URL(path, backendOrigin).toString();
}
