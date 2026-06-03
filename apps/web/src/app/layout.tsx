import type { Metadata } from "next";
import { Inter, Cormorant_Garamond, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { Providers } from "@/providers";
import { WebVitalsReporter } from "@/components/layout/web-vitals-reporter";

const inter = Inter({ subsets: ["latin"], variable: "--font-inter", display: "swap" });
// High-contrast editorial serif for display/headings (frontend-design skill pick).
// Cormorant is delicate, so load mid-heavy weights + italic and use at large sizes.
const cormorant = Cormorant_Garamond({
  subsets: ["latin"],
  weight: ["500", "600", "700"],
  style: ["normal", "italic"],
  variable: "--font-serif-display",
  display: "swap",
});
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-jetbrains", display: "swap" });

export const metadata: Metadata = {
  title: "SRapi — Self-hosted AI gateway",
  description: "One endpoint, every provider, your accounts, your control.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh" suppressHydrationWarning>
      <body
        className={`${inter.variable} ${cormorant.variable} ${mono.variable} paper-grain min-h-dvh antialiased`}
      >
        <Providers>{children}</Providers>
        <WebVitalsReporter />
      </body>
    </html>
  );
}
