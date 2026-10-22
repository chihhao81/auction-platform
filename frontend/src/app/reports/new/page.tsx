import { Suspense } from "react";
import { RequireMember } from "@/lib/auth";
import { ReportForm } from "./ReportForm";

export default function NewReportPage() {
  return (
    <RequireMember>
      <Suspense fallback={null}>
        <ReportForm />
      </Suspense>
    </RequireMember>
  );
}
