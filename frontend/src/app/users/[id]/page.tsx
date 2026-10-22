import { Suspense } from "react";
import { UserProfileView } from "./UserProfileView";

export default function UserPage() {
  return (
    <Suspense fallback={<p className="text-center text-gray-500">載入中…</p>}>
      <UserProfileView />
    </Suspense>
  );
}
