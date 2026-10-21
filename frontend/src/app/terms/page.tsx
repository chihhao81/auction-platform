import { Suspense } from "react";
import { TermsView } from "./TermsView";

export default function TermsPage() {
  return (
    <Suspense fallback={null}>
      <TermsView />
    </Suspense>
  );
}
