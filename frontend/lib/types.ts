export type ClaimStatus = "PENDING" | "APPROVED" | "REVIEW_REQUIRED" | "REJECTED";

export interface Claimant {
  id: string;
  fullName: string;
  email: string;
  active: boolean;
  createdAt: string;
}

export interface Claim {
  id: string;
  claimantId: string;
  policyNumber: string;
  incidentDate: string;
  amountCents: number;
  currency: string;
  description: string;
  status: ClaimStatus;
  decisionReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ClaimList {
  items: Claim[];
  count: number;
}

export interface APIProblem {
  status: number;
  title: string;
  detail: string;
}
