import type { APIProblem, Claim, Claimant, ClaimList } from "./types";

const apiRoot = "/api/oc";

export class APIError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiRoot}${path}`, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
    cache: "no-store",
  });

  if (!response.ok) {
    let detail = `Request failed with status ${response.status}`;
    try {
      const problem = (await response.json()) as APIProblem;
      detail = problem.detail || detail;
    } catch {
      // Keep the status-based fallback when the response is not JSON.
    }
    throw new APIError(detail, response.status);
  }

  return (await response.json()) as T;
}

export const api = {
  health: () => request<{ status: string }>("/health/ready"),
  createClaimant: (input: { fullName: string; email: string }) =>
    request<Claimant>("/v1/claimants", { method: "POST", body: JSON.stringify(input) }),
  getClaimant: (id: string) => request<Claimant>(`/v1/claimants/${encodeURIComponent(id)}`),
  listClaims: (claimantID: string) =>
    request<ClaimList>(`/v1/claimants/${encodeURIComponent(claimantID)}/claims`),
  createClaim: (input: {
    claimantId: string;
    policyNumber: string;
    incidentDate: string;
    amountCents: number;
    currency: string;
    description: string;
    idempotencyKey: string;
  }) => request<Claim>("/v1/claims", { method: "POST", body: JSON.stringify(input) }),
};
