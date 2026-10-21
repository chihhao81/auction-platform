import { Suspense } from "react";
import { HomeView } from "./HomeView";

export default function HomePage() {
  return (
    <Suspense fallback={<p className="text-center text-gray-500">載入中…</p>}>
      <HomeView />
    </Suspense>
  );
}
