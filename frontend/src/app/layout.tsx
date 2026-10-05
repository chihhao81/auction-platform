import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
	title: "小島競標｜讓喜歡相遇",
	description: "為特殊寵物與收藏愛好者打造的個人競標平台。",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-Hant"><body>{children}</body></html>;
}
