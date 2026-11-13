"use client";

import { useEffect, useMemo, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { api, APIError } from "@/lib/api";
import type { Claim, Claimant, ClaimantSummary, ClaimStatus } from "@/lib/types";

type View = "overview" | "claimants" | "register" | "submit" | "account";
type Notice = { tone: "success" | "error" | "warning" | "info"; message: string } | null;

const navigation: Array<{ id: View; label: string; icon: IconName }> = [
  { id: "overview", label: "Overview", icon: "grid" },
  { id: "claimants", label: "Claimants", icon: "people" },
  { id: "register", label: "Register claimant", icon: "person" },
  { id: "submit", label: "Submit a claim", icon: "file" },
  { id: "account", label: "Claim history", icon: "clock" },
];

export function ClaimsPortal() {
  const [view, setView] = useState<View>("overview");
  const [health, setHealth] = useState<"checking" | "online" | "offline">("checking");
  const [claimant, setClaimant] = useState<Claimant | null>(null);
  const [claims, setClaims] = useState<Claim[]>([]);
  const [notice, setNotice] = useState<Notice>(null);
  const [busy, setBusy] = useState(false);
  const [claimantPrompt, setClaimantPrompt] = useState(false);

  async function loadAccount(id: string, destination: View | null = "account") {
    const cleanID = id.trim();
    if (!cleanID) return;
    setBusy(true);
    setNotice(null);
    try {
      const [nextClaimant, history] = await Promise.all([
        api.getClaimant(cleanID),
        api.listClaims(cleanID),
      ]);
      setClaimant(nextClaimant);
      setClaims(history.items);
      setClaimantPrompt(false);
      window.localStorage.setItem("oc-broker.claimant-id", nextClaimant.id);
      if (destination) setView(destination);
    } catch (error) {
      setNotice({ tone: "error", message: getErrorMessage(error) });
      if (destination) setView("claimants");
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    api.health().then(() => setHealth("online")).catch(() => setHealth("offline"));
    const savedClaimantID = window.localStorage.getItem("oc-broker.claimant-id");
    if (savedClaimantID) void loadAccount(savedClaimantID, null);
  }, []);

  const hasPendingClaims = claims.some((claim) => claim.status === "PENDING");

  useEffect(() => {
    if (!claimant || !hasPendingClaims) return;
    const interval = window.setInterval(() => {
      api.listClaims(claimant.id).then((history) => setClaims(history.items)).catch(() => undefined);
    }, 2500);
    return () => window.clearInterval(interval);
  }, [claimant, hasPendingClaims]);

  async function registerClaimant(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    setBusy(true);
    setNotice(null);
    const form = new FormData(formElement);
    try {
      const created = await api.createClaimant({
        fullName: String(form.get("fullName") ?? ""),
        email: String(form.get("email") ?? ""),
      });
      setClaimant(created);
      setClaims([]);
      setClaimantPrompt(false);
      window.localStorage.setItem("oc-broker.claimant-id", created.id);
      formElement.reset();
      setNotice({ tone: "success", message: `${created.fullName} is registered and ready to make a claim.` });
      setView("submit");
    } catch (error) {
      setNotice({ tone: "error", message: getErrorMessage(error) });
    } finally {
      setBusy(false);
    }
  }

  async function submitClaim(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    if (!claimant) {
      setNotice({ tone: "info", message: "Register or select a claimant before submitting a claim." });
      setView("register");
      return;
    }
    setBusy(true);
    setNotice(null);
    const form = new FormData(formElement);
    const amount = Number(form.get("amount"));
    try {
      const created = await api.createClaim({
        claimantId: claimant.id,
        policyNumber: String(form.get("policyNumber") ?? ""),
        incidentDate: String(form.get("incidentDate") ?? ""),
        amountCents: Math.round(amount * 100),
        currency: String(form.get("currency") ?? "GBP"),
        description: String(form.get("description") ?? ""),
        idempotencyKey: crypto.randomUUID(),
      });
      setClaims((current) => [created, ...current.filter((claim) => claim.id !== created.id)]);
      formElement.reset();
      setNotice({ tone: "success", message: "Claim received. We are checking it now." });
      setView("account");
    } catch (error) {
      setNotice({ tone: "error", message: getErrorMessage(error) });
    } finally {
      setBusy(false);
    }
  }

  function navigate(nextView: View) {
    setNotice(null);
    if (nextView === "submit" && !claimant) {
      setNotice({ tone: "warning", message: "A claimant account is required before a claim can be submitted." });
      setClaimantPrompt(true);
      setView("register");
      return;
    }
    if (nextView === "account" && !claimant) {
      setView("claimants");
      return;
    }
    setClaimantPrompt(false);
    setView(nextView);
  }

  function clearAccount() {
    window.localStorage.removeItem("oc-broker.claimant-id");
    setClaimant(null);
    setClaims([]);
    setNotice(null);
    setView("claimants");
  }

  const approvedCount = claims.filter((claim) => claim.status === "APPROVED").length;
  const reviewCount = claims.filter((claim) => claim.status === "REVIEW_REQUIRED").length;

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true">OC</div>
          <div><strong>OC Broker</strong><span>Claims workspace</span></div>
        </div>

        <nav className="main-nav" aria-label="Main navigation">
          <p className="nav-label">Workspace</p>
          {navigation.map((item) => (
            <button
              className={`${view === item.id ? "nav-item active" : "nav-item"}${item.id === "submit" && !claimant ? " locked" : ""}`}
              disabled={item.id === "submit" && !claimant}
              key={item.id}
              onClick={() => navigate(item.id)}
              title={item.id === "submit" && !claimant ? "Register or select a claimant first" : undefined}
              type="button"
            >
              <Icon name={item.icon} />
              <span>{item.label}</span>
            </button>
          ))}
          {!claimant && <p className="nav-prerequisite"><Icon name="info" /> Register or select a claimant to unlock claim submission.</p>}
        </nav>

        <div className="sidebar-foot">
          <div className="system-card">
            <div className="system-card-icon"><Icon name="shield" /></div>
            <strong>Secure workspace</strong>
            <p>Your claims are protected for peace of mind.</p>
          </div>
          <p className="version">OC Broker</p>
        </div>
      </aside>

      <main className="main-content">
        <header className="topbar">
          <div>
            <p className="eyebrow">Claims operations</p>
            <h1>{viewTitle(view)}</h1>
          </div>
          <div className={`health-pill ${health}`} title="Go API readiness">
            <span className="health-dot" />
            {health === "online" ? "All systems ready" : health === "offline" ? "Service unavailable" : "Checking systems"}
          </div>
        </header>

        {notice && (
          <div className={`notice ${notice.tone}`} role={notice.tone === "error" || notice.tone === "warning" ? "alert" : "status"}>
            <Icon name={notice.tone === "error" || notice.tone === "warning" ? "alert" : notice.tone === "success" ? "check" : "info"} />
            <span>{notice.message}</span>
            <button type="button" onClick={() => setNotice(null)} aria-label="Dismiss message">×</button>
          </div>
        )}

        {view === "overview" && (
          <Overview
            claimant={claimant}
            claims={claims}
            approvedCount={approvedCount}
            reviewCount={reviewCount}
            onNavigate={navigate}
          />
        )}
        {view === "claimants" && (
          <ClaimantsView
            onOpen={(id) => loadAccount(id, "account")}
            onRegister={() => navigate("register")}
            onSubmit={(id) => loadAccount(id, "submit")}
            selectedClaimantID={claimant?.id}
          />
        )}
        {view === "register" && <RegisterView busy={busy} prompted={claimantPrompt} onSubmit={registerClaimant} onBrowse={() => navigate("claimants")} />}
        {view === "submit" && claimant && <SubmitView claimant={claimant} busy={busy} onSubmit={submitClaim} />}
        {view === "account" && (
          <AccountView claimant={claimant} claims={claims} onBrowse={() => navigate("claimants")} onSubmit={() => navigate("submit")} onSwitch={clearAccount} />
        )}
      </main>
    </div>
  );
}

function Overview({ claimant, claims, approvedCount, reviewCount, onNavigate }: {
  claimant: Claimant | null;
  claims: Claim[];
  approvedCount: number;
  reviewCount: number;
  onNavigate: (view: View) => void;
}) {
  const pendingCount = claims.filter((claim) => claim.status === "PENDING").length;
  return (
    <div className="view-stack">
      <section className="hero-card">
        <div className="hero-copy">
          <span className="hero-kicker"><Icon name="spark" /> Built for calmer claims</span>
          <h2>From first report to a clear decision.</h2>
          <p>Register a claimant, capture the incident and let us process and handle the next step.</p>
          <div className="hero-actions">
            <button className="button primary" type="button" onClick={() => onNavigate(claimant ? "submit" : "register")}>
              <Icon name="plus" /> {claimant ? "Start a new claim" : "Register a claimant"}
            </button>
            <button className="button secondary" type="button" onClick={() => onNavigate("account")}>
              View claim history
            </button>
          </div>
        </div>
        <div className="hero-visual" aria-hidden="true">
          <div className="orbit orbit-one" />
          <div className="orbit orbit-two" />
          <div className="decision-card">
            <span className="decision-icon"><Icon name="check" /></span>
            <div><small>Decision engine</small><strong>Ready to process</strong></div>
          </div>
          <div className="mini-card"><span>01</span><p>Capture</p></div>
          <div className="mini-card second"><span>02</span><p>Decide</p></div>
        </div>
      </section>

      <section className="stat-grid" aria-label="Claim summary">
        <StatCard label="Total claims" value={claims.length} icon="file" tone="navy" />
        <StatCard label="Approved" value={approvedCount} icon="check" tone="green" />
        <StatCard label="In progress" value={pendingCount} icon="clock" tone="amber" />
        <StatCard label="Needs review" value={reviewCount} icon="eye" tone="coral" />
      </section>

      <section className="dashboard-grid">
        <div className="panel activity-panel">
          <PanelHeading title="Recent claims" subtitle="Latest activity for the selected claimant" action={claims.length ? "View all" : undefined} onAction={() => onNavigate("account")} />
          {claims.length ? (
            <div className="claim-list compact">
              {claims.slice(0, 4).map((claim) => <ClaimRow claim={claim} key={claim.id} />)}
            </div>
          ) : (
            <EmptyState icon="file" title="No claims yet" text="New claims will appear here as soon as they are submitted." />
          )}
        </div>

        <div className="panel claimant-panel">
          <PanelHeading title="Current claimant" subtitle="The account for this browser session" />
          {claimant ? (
            <>
              <div className="person-summary">
                <div className="avatar">{initials(claimant.fullName)}</div>
                <div><strong>{claimant.fullName}</strong><span>{claimant.email}</span></div>
                <span className="active-badge">Active</span>
              </div>
              <dl className="detail-list">
                <div><dt>Claimant ID</dt><dd className="claimant-id-full">{claimant.id}</dd></div>
                <div><dt>Registered</dt><dd>{formatDate(claimant.createdAt)}</dd></div>
              </dl>
              <button className="text-button" type="button" onClick={() => onNavigate("account")}>Open account <span>→</span></button>
            </>
          ) : (
            <EmptyState icon="person" title="No claimant selected" text="Register a claimant or choose an existing account from the directory." />
          )}
        </div>
      </section>
    </div>
  );
}

const claimantPageSize = 10;

function ClaimantsView({ onOpen, onRegister, onSubmit, selectedClaimantID }: {
  onOpen: (id: string) => Promise<void>;
  onRegister: () => void;
  onSubmit: (id: string) => Promise<void>;
  selectedClaimantID?: string;
}) {
  const [draftSearch, setDraftSearch] = useState("");
  const [search, setSearch] = useState("");
  const [offset, setOffset] = useState(0);
  const [items, setItems] = useState<ClaimantSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectingID, setSelectingID] = useState("");

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    api.listClaimants({ search, limit: claimantPageSize, offset })
      .then((page) => {
        if (!active) return;
        setItems(page.items);
        setTotal(page.total);
      })
      .catch((cause) => {
        if (!active) return;
        setItems([]);
        setTotal(0);
        setError(getErrorMessage(cause));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [search, offset]);

  function applySearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setOffset(0);
    setSearch(draftSearch.trim());
  }

  function clearSearch() {
    setDraftSearch("");
    setSearch("");
    setOffset(0);
  }

  async function selectClaimant(id: string, action: (claimantID: string) => Promise<void>) {
    setSelectingID(id);
    try {
      await action(id);
    } finally {
      setSelectingID("");
    }
  }

  const firstResult = total === 0 ? 0 : offset + 1;
  const lastResult = Math.min(offset + claimantPageSize, total);

  return (
    <div className="view-stack">
      <section className="panel directory-panel">
        <div className="directory-heading">
          <div>
            <p className="eyebrow">Claimant records</p>
            <h2>Find the person you are helping.</h2>
            <p>Open an account or start a claim without copying a claimant ID.</p>
          </div>
          <button className="button primary" type="button" onClick={onRegister}><Icon name="person" /> Register claimant</button>
        </div>

        <form className="directory-search" role="search" onSubmit={applySearch}>
          <label>
            <span className="search-icon"><Icon name="search" /></span>
            <input
              aria-label="Search claimants by name or email"
              maxLength={100}
              onChange={(event) => setDraftSearch(event.target.value)}
              placeholder="Search by name or email"
              type="search"
              value={draftSearch}
            />
          </label>
          <button className="button secondary" type="submit">Search</button>
          {search && <button className="text-button clear-search" type="button" onClick={clearSearch}>Clear</button>}
        </form>

        <div className="directory-meta" aria-live="polite">
          <span>{loading ? "Loading claimants…" : search ? `${total} result${total === 1 ? "" : "s"} for “${search}”` : `${total} registered claimant${total === 1 ? "" : "s"}`}</span>
          {!loading && total > 0 && <span>Showing {firstResult}–{lastResult}</span>}
        </div>

        {error ? (
          <div className="directory-error" role="alert"><Icon name="alert" /><div><strong>Could not load claimants</strong><p>{error}</p></div></div>
        ) : loading ? (
          <div className="directory-loading" role="status"><Spinner /> Loading claimant directory</div>
        ) : items.length ? (
          <div className="claimant-directory-list">
            {items.map((item) => (
              <article className={`claimant-directory-row${item.id === selectedClaimantID ? " selected" : ""}`} key={item.id}>
                <div className="avatar">{initials(item.fullName)}</div>
                <div className="directory-person">
                  <div><strong>{item.fullName}</strong>{item.id === selectedClaimantID && <span className="selected-badge">Selected</span>}</div>
                  <span>{item.email}</span>
                </div>
                <div className="directory-fact claim-count"><span>Claims</span><strong>{item.claimCount}</strong></div>
                <div className="directory-fact latest-status"><span>Latest status</span>{item.latestClaimStatus ? <StatusBadge status={item.latestClaimStatus} /> : <strong>None yet</strong>}</div>
                <div className="directory-fact last-activity"><span>Last activity</span><strong>{formatDate(item.lastActivityAt)}</strong></div>
                <div className="directory-actions">
                  <button className="button secondary" disabled={Boolean(selectingID)} type="button" onClick={() => void selectClaimant(item.id, onOpen)}>{selectingID === item.id ? "Opening…" : "Open account"}</button>
                  <button className="icon-button" disabled={Boolean(selectingID)} type="button" onClick={() => void selectClaimant(item.id, onSubmit)} aria-label={`Submit a claim for ${item.fullName}`} title="Submit a claim"><Icon name="plus" /></button>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState
            icon="people"
            title={search ? "No matching claimants" : "No claimants registered"}
            text={search ? "Try a different name or email address." : "Register the first claimant to begin processing claims."}
            action={search ? "Clear search" : "Register claimant"}
            onAction={search ? clearSearch : onRegister}
          />
        )}

        {!loading && !error && total > claimantPageSize && (
          <div className="pagination" aria-label="Claimant directory pages">
            <button className="button secondary" disabled={offset === 0} type="button" onClick={() => setOffset(Math.max(0, offset - claimantPageSize))}>← Previous</button>
            <span>Page {Math.floor(offset / claimantPageSize) + 1} of {Math.ceil(total / claimantPageSize)}</span>
            <button className="button secondary" disabled={offset + claimantPageSize >= total} type="button" onClick={() => setOffset(offset + claimantPageSize)}>Next →</button>
          </div>
        )}
      </section>
    </div>
  );
}

function RegisterView({ busy, prompted, onSubmit, onBrowse }: {
  busy: boolean;
  prompted: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onBrowse: () => void;
}) {
  return (
    <div className="split-layout">
      <section className={prompted ? "panel form-panel prompted" : "panel form-panel"}>
        <div className="step-number">01</div>
        <p className="eyebrow">New account</p>
        <h2>Tell us who is making the claim.</h2>
        <p className="form-intro">We only need the essentials to create a claimant record.</p>
        {prompted && (
          <div className="requirement-note" role="status">
            <span><Icon name="person" /></span>
            <div><strong>Claimant required</strong><p>Register someone below, or choose an existing claimant from the directory.</p></div>
          </div>
        )}
        <form onSubmit={onSubmit} className="form-grid">
          <label className="field full"><span>Full name</span><input name="fullName" autoComplete="name" minLength={2} placeholder="e.g. Alex Morgan" required /></label>
          <label className="field full"><span>Email address</span><input name="email" type="email" autoComplete="email" placeholder="alex@example.com" required /></label>
          <div className="form-actions full">
            <button className="button primary" disabled={busy} type="submit">{busy ? <Spinner /> : <Icon name="person" />} Create claimant</button>
            <span className="form-hint"><Icon name="shield" /> Stored securely</span>
          </div>
        </form>
      </section>

      <aside className="side-stack">
        <section className={prompted ? "panel lookup-card prompted" : "panel lookup-card"}>
          <span className="side-icon"><Icon name="search" /></span>
          <h3>Already registered?</h3>
          <p>Search the claimant directory by name or email to restore their account and claim history.</p>
          <button className="button secondary wide" type="button" onClick={onBrowse}><Icon name="people" /> Browse claimants</button>
        </section>
        <section className="guidance-card">
          <Icon name="info" />
          <div><strong>What happens next?</strong><p>After registration, you’ll move directly to the claim form.</p></div>
        </section>
      </aside>
    </div>
  );
}

function SubmitView({ claimant, busy, onSubmit }: { claimant: Claimant; busy: boolean; onSubmit: (event: FormEvent<HTMLFormElement>) => void }) {
  const today = new Date().toISOString().slice(0, 10);
  return (
    <div className="split-layout claim-form-layout">
      <section className="panel form-panel">
        <div className="step-number">02</div>
        <p className="eyebrow">First notification of loss</p>
        <h2>Capture what happened.</h2>
        <p className="form-intro">Give the processing service enough detail to make its initial decision.</p>
        <form onSubmit={onSubmit} className="form-grid two-columns">
          <label className="field"><span>Policy number</span><input name="policyNumber" minLength={3} placeholder="OC-2026-001" required /></label>
          <label className="field"><span>Incident date</span><input name="incidentDate" type="date" max={today} required /></label>
          <label className="field amount-field"><span>Claim amount</span><div className="input-combo"><select name="currency" aria-label="Currency" defaultValue="GBP"><option>GBP</option><option>EUR</option><option>USD</option></select><input name="amount" type="number" min="0.01" step="0.01" inputMode="decimal" placeholder="0.00" required /></div></label>
          <div className="claimant-readonly"><span>Claimant</span><div className="mini-person"><div className="avatar small">{initials(claimant.fullName)}</div><div><strong>{claimant.fullName}</strong><small className="claimant-id-full">{claimant.id}</small></div><CopyIDButton id={claimant.id} compact /></div></div>
          <label className="field full"><span>What happened?</span><textarea name="description" minLength={10} rows={5} placeholder="Describe the incident and the damage in a few sentences…" required /><small>At least 10 characters</small></label>
          <div className="form-actions full">
            <button className="button primary" disabled={busy} type="submit">{busy ? <Spinner /> : <Icon name="send" />} Submit claim</button>
            <span className="form-hint"><Icon name="spark" /> Usually processed in a few seconds</span>
          </div>
        </form>
      </section>

      <aside className="side-stack">
        <section className="panel process-card">
          <p className="eyebrow">How it works</p>
          <h3>One form, three reliable steps.</h3>
          <ol className="process-list">
            <li className="current"><span>1</span><div><strong>Claim recorded</strong><p>Saved with a unique submission key.</p></div></li>
            <li><span>2</span><div><strong>Sent for processing</strong><p>The outbox publishes a command to SQS.</p></div></li>
            <li><span>3</span><div><strong>Decision returned</strong><p>Status updates automatically in history.</p></div></li>
          </ol>
        </section>
      </aside>
    </div>
  );
}

function AccountView({ claimant, claims, onBrowse, onSubmit, onSwitch }: {
  claimant: Claimant | null;
  claims: Claim[];
  onBrowse: () => void;
  onSubmit: () => void;
  onSwitch: () => void;
}) {
  const totalValue = useMemo(() => claims.reduce((sum, claim) => sum + claim.amountCents, 0), [claims]);
  const singleCurrency = claims.length > 0 && claims.every((claim) => claim.currency === claims[0].currency);
  if (!claimant) {
    return (
      <section className="panel centered-panel">
        <div className="large-icon"><Icon name="search" /></div>
        <p className="eyebrow">Claimant account</p>
        <h2>Select a claimant first.</h2>
        <p>Use the directory to find an account by name or email.</p>
        <button className="button primary" type="button" onClick={onBrowse}><Icon name="people" /> Browse claimants</button>
      </section>
    );
  }

  return (
    <div className="view-stack">
      <section className="account-header panel">
        <div className="person-summary large">
          <div className="avatar large">{initials(claimant.fullName)}</div>
          <div><span className="active-badge">Active claimant</span><h2>{claimant.fullName}</h2><p>{claimant.email}</p></div>
        </div>
        <div className="account-actions">
          <button className="button secondary" type="button" onClick={onSwitch}><Icon name="switch" /> Switch claimant</button>
          <button className="button primary" type="button" onClick={onSubmit}><Icon name="plus" /> New claim</button>
        </div>
      </section>

      <section className="account-metrics">
        <div><span>Claims on record</span><strong>{claims.length}</strong></div>
        <div><span>Total claimed</span><strong>{singleCurrency ? formatMoney(totalValue, claims[0].currency) : claims.length ? "Mixed currencies" : "—"}</strong></div>
        <div><span>Member since</span><strong>{formatDate(claimant.createdAt)}</strong></div>
        <div className="account-id-metric"><span>Claimant ID</span><strong className="claimant-id-full">{claimant.id}</strong><CopyIDButton id={claimant.id} /></div>
      </section>

      <section className="panel history-panel">
        <PanelHeading title="Claim history" subtitle={claims.length ? `${claims.length} claim${claims.length === 1 ? "" : "s"}, newest first` : "No claims recorded"} />
        {claims.length ? <div className="claim-list">{claims.map((claim) => <ClaimRow claim={claim} detailed key={claim.id} />)}</div> : <EmptyState icon="file" title="A clean slate" text="When a claim is submitted, its live status will appear here." action="Submit first claim" onAction={onSubmit} />}
      </section>
    </div>
  );
}

function ClaimRow({ claim, detailed = false }: { claim: Claim; detailed?: boolean }) {
  return (
    <article className={detailed ? "claim-row detailed" : "claim-row"}>
      <div className="claim-icon"><Icon name={claim.status === "APPROVED" ? "check" : claim.status === "REVIEW_REQUIRED" ? "eye" : "clock"} /></div>
      <div className="claim-primary"><strong>{claim.policyNumber}</strong><span>{formatDate(claim.createdAt)} · {shortID(claim.id)}</span>{detailed && <p>{claim.description}</p>}</div>
      {detailed && <div className="incident"><span>Incident</span><strong>{formatDate(claim.incidentDate)}</strong></div>}
      <div className="claim-value"><strong>{formatMoney(claim.amountCents, claim.currency)}</strong><StatusBadge status={claim.status} /></div>
    </article>
  );
}

function StatusBadge({ status }: { status: ClaimStatus }) {
  return <span className={`status-badge ${status.toLowerCase()}`}><span />{statusLabel(status)}</span>;
}

function StatCard({ label, value, icon, tone }: { label: string; value: number; icon: IconName; tone: string }) {
  return <div className="stat-card"><span className={`stat-icon ${tone}`}><Icon name={icon} /></span><div><strong>{value.toString().padStart(2, "0")}</strong><span>{label}</span></div></div>;
}

function PanelHeading({ title, subtitle, action, onAction }: { title: string; subtitle: string; action?: string; onAction?: () => void }) {
  return <div className="panel-heading"><div><h3>{title}</h3><p>{subtitle}</p></div>{action && <button type="button" onClick={onAction}>{action} <span>→</span></button>}</div>;
}

function EmptyState({ icon, title, text, action, onAction }: { icon: IconName; title: string; text: string; action?: string; onAction?: () => void }) {
  return <div className="empty-state"><span><Icon name={icon} /></span><strong>{title}</strong><p>{text}</p>{action && <button className="text-button" type="button" onClick={onAction}>{action} →</button>}</div>;
}

function Spinner() { return <span className="spinner" aria-label="Working" />; }

function CopyIDButton({ id, compact = false }: { id: string; compact?: boolean }) {
  const [copied, setCopied] = useState(false);

  async function copyID() {
    try {
      await navigator.clipboard.writeText(id);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      window.prompt("Copy claimant ID", id);
    }
  }

  return (
    <button className={compact ? "copy-id compact" : "copy-id"} type="button" onClick={copyID} aria-label="Copy full claimant ID">
      <Icon name={copied ? "check" : "copy"} /> {compact ? "" : copied ? "Copied" : "Copy ID"}
    </button>
  );
}

type IconName = "grid" | "person" | "people" | "file" | "clock" | "shield" | "alert" | "check" | "info" | "spark" | "plus" | "eye" | "search" | "send" | "switch" | "copy";

function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactNode> = {
    grid: <><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></>,
    person: <><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></>,
    people: <><circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0M16 5.5a3 3 0 0 1 0 5.8M17 14a5.5 5.5 0 0 1 4.5 5.5"/></>,
    file: <><path d="M6 2h8l4 4v16H6z"/><path d="M14 2v5h5M9 13h6M9 17h6"/></>,
    clock: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></>,
    shield: <><path d="M12 2 4.5 5v6c0 5 3 8.5 7.5 11 4.5-2.5 7.5-6 7.5-11V5z"/><path d="m9 12 2 2 4-5"/></>,
    alert: <><path d="M12 3 2.8 20h18.4z"/><path d="M12 9v5M12 17.5v.1"/></>,
    check: <path d="m5 12 4 4L19 6"/>,
    info: <><circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.1"/></>,
    spark: <path d="m12 2 1.5 6.5L20 10l-6.5 1.5L12 18l-1.5-6.5L4 10l6.5-1.5z"/>,
    plus: <path d="M12 5v14M5 12h14"/>,
    eye: <><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6Z"/><circle cx="12" cy="12" r="2.5"/></>,
    search: <><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/></>,
    send: <><path d="m3 11 18-8-7 18-3-7z"/><path d="m11 14 10-11"/></>,
    switch: <><path d="M7 7h11l-3-3M17 17H6l3 3"/></>,
    copy: <><rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/></>,
  };
  return <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}

function getErrorMessage(error: unknown) {
  if (error instanceof APIError) return error.message;
  return "Something went wrong. Please try again.";
}

function viewTitle(view: View) {
  return { overview: "Good morning", claimants: "Claimants", register: "Register a claimant", submit: "Submit a claim", account: "Claimant account" }[view];
}

function initials(name: string) { return name.split(/\s+/).slice(0, 2).map((part) => part[0]).join("").toUpperCase(); }
function shortID(id: string) { return `${id.slice(0, 8)}…${id.slice(-4)}`; }
function formatDate(value: string) { return new Intl.DateTimeFormat("en-GB", { day: "2-digit", month: "short", year: "numeric" }).format(new Date(value)); }
function formatMoney(cents: number, currency: string) { return new Intl.NumberFormat("en-GB", { style: "currency", currency }).format(cents / 100); }
function statusLabel(status: ClaimStatus) { return { PENDING: "Processing", APPROVED: "Approved", REVIEW_REQUIRED: "Needs review", REJECTED: "Rejected" }[status]; }
