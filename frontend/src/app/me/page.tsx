import { RequireMember } from "@/lib/auth";
import { ProfileView } from "./ProfileView";

export default function MePage() {
  return (
    <RequireMember>
      <ProfileView />
    </RequireMember>
  );
}
