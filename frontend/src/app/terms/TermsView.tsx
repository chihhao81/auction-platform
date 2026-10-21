"use client";

import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ApiError, api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Terms } from "@/lib/types";

export function TermsView() {
  const { state, refresh } = useAuth();
  const router = useRouter();
  const next = useSearchParams().get("next") ?? "/";
  const [terms, setTerms] = useState<Terms | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    api.get<Terms>("/api/terms").then(setTerms, () => setError("無法載入條款"));
  }, []);

  const accepted = state.status === "authenticated" && state.me.terms.accepted;

  async function accept() {
    if (!terms) return;
    setSubmitting(true);
    setError(null);
    try {
      await api.post("/api/terms/accept", { version: terms.version });
      await refresh();
      // Only follow same-origin relative paths.
      router.replace(next.startsWith("/") && !next.startsWith("//") ? next : "/");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "同意條款失敗");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl rounded-xl bg-white p-6 shadow">
      {terms ? (
        <>
          <p className="mb-2 text-xs text-gray-500">版本 {terms.version}</p>
          {/* Rendered as plain text: React escapes it, no HTML is interpreted. */}
          <div className="whitespace-pre-wrap text-sm leading-7">{terms.content}</div>
        </>
      ) : (
        <p className="text-gray-500">載入中…</p>
      )}
      {error && <p role="alert" className="mt-4 text-sm text-red-600">{error}</p>}
      {state.status === "authenticated" && terms && (
        <div className="mt-6 border-t pt-4">
          {accepted ? (
            <p className="text-sm text-emerald-700">您已同意此版本條款。</p>
          ) : (
            <button
              onClick={accept}
              disabled={submitting}
              className="w-full rounded bg-emerald-600 px-4 py-2 font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
            >
              {submitting ? "處理中…" : "我已閱讀並同意"}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
