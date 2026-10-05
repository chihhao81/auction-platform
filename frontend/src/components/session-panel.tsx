"use client";

import { useEffect, useState } from "react";
import { apiURL } from "@/lib/api";
import { acceptTerms as submitTerms, logout as logoutSession, readSession, type Terms, type User } from "@/lib/session";
type View = "loading" | "anonymous" | "terms" | "signed-in" | "error";

export function SessionPanel() {
  const [view, setView] = useState<View>("loading");
  const [terms, setTerms] = useState<Terms | null>(null);
  const [user, setUser] = useState<User | null>(null);
  const [accepted, setAccepted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [loginNotice, setLoginNotice] = useState("");

  async function loadSession() {
    try {
      const snapshot = await readSession();
      if (snapshot.kind === "anonymous") setView("anonymous");
      if (snapshot.kind === "terms-required") {
        setTerms(snapshot.terms);
        setView("terms");
      }
      if (snapshot.kind === "signed-in") {
        setUser(snapshot.user);
        setView("signed-in");
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "連線失敗，請稍後再試。");
      setView("error");
    }
  }

  useEffect(() => {
    if (new URLSearchParams(window.location.search).get("login") === "cancelled") {
      setLoginNotice("你已取消 LINE 登入，隨時可以重新開始。");
    }
    void loadSession();
  }, []);

  async function acceptTerms() {
    if (!terms || !accepted) return;
    setBusy(true);
    setMessage("");
    try {
      await submitTerms(terms.version);
      await loadSession();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "連線失敗，請稍後再試。");
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    setBusy(true);
    try {
      await logoutSession();
      setUser(null);
      setMessage("");
      setView("anonymous");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "登出失敗。");
    } finally {
      setBusy(false);
    }
  }

  if (view === "loading") return <div className="sessionPanel" aria-live="polite"><div className="sessionLoading" /><p className="sessionIntro">正在安全確認你的登入狀態…</p></div>;
  if (view === "anonymous") return <div className="sessionPanel" aria-live="polite">
    <a className="cta" href={apiURL("/v1/auth/line")}>使用 LINE 登入 <span aria-hidden="true">↗</span></a>
    <p className="sessionIntro">首次登入會建立平台帳戶，並請你閱讀及確認使用者條款。</p>
    {loginNotice && <p className="sessionMessage" role="status">{loginNotice}</p>}
  </div>;
  if (view === "terms" && terms) return (
    <section className="termsBox" aria-labelledby="terms-heading">
      <h2 id="terms-heading">請閱讀並接受使用者條款（{terms.version}）</h2>
      <div className="termsText">{terms.text}</div>
      <label className="termsAccept"><input type="checkbox" checked={accepted} onChange={(event) => setAccepted(event.target.checked)} />我已閱讀並同意上述使用者條款。</label>
      <button className="termsSubmit" type="button" disabled={!accepted || busy} onClick={() => void acceptTerms()}>{busy ? "儲存中…" : "同意並繼續"}</button>
      {message && <p className="sessionMessage" role="alert">{message}</p>}
    </section>
  );
  if (view === "signed-in" && user) return <div className="sessionPanel signedInPanel" aria-live="polite">
    <div className="profileLine">
      <span className="profileAvatar">{user.avatar_url ? <img src={user.avatar_url} alt="" /> : (user.display_name.slice(0, 1) || "島")}</span>
      <span className="profileMeta"><strong>{user.display_name}</strong><span>LINE 帳戶已連結</span></span>
    </div>
    <button className="termsSubmit" type="button" disabled={busy} onClick={() => void logout()}>{busy ? "處理中…" : "登出帳戶"}</button>
    {message && <p className="sessionMessage" role="alert">{message}</p>}
  </div>;
  return <div className="sessionPanel" role="alert"><p>{message || "目前無法連線至平台服務。"}</p><button className="termsSubmit" type="button" onClick={() => { setView("loading"); void loadSession(); }}>重新連線</button></div>;
}
