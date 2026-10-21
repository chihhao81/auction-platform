import { RequireMember } from "@/lib/auth";
import { NotificationList } from "./NotificationList";

export default function NotificationsPage() {
  return (
    <RequireMember>
      <NotificationList />
    </RequireMember>
  );
}
