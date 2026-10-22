import { RequireMember } from "@/lib/auth";
import { AdminConsole } from "./AdminConsole";

export default function AdminPage() {
  return (
    <RequireMember>
      <AdminConsole />
    </RequireMember>
  );
}
