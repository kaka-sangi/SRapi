import type { Metadata } from "next";
import { headers } from "next/headers";
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

// Render every route per-request rather than prerendering it at build time.
// The proxy (src/proxy.ts) issues a fresh CSP nonce per request and Next can
// only stamp that nonce onto its inline bootstrap scripts while rendering
// dynamically — a build-time static prerender would bake nonce-less scripts the
// browser then blocks, so the console would load but never hydrate. This is an
// authenticated client console (no SEO/static-cache need), so the cost is nil.
export const dynamic = "force-dynamic";

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  // The proxy's per-request nonce (src/proxy.ts). next-themes injects a pre-paint
  // inline script that Next does not stamp, so it would be CSP-blocked without
  // this — pass it through so the theme applies before first paint.
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return (
    <html lang="zh" suppressHydrationWarning>
      <body
        className={`${inter.variable} ${cormorant.variable} ${mono.variable} paper-grain min-h-dvh antialiased`}
      >
        <Providers nonce={nonce}>{children}</Providers>
        <WebVitalsReporter />
      </body>
    </html>
  );
}
