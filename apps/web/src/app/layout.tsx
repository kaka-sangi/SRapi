import type { Metadata, Viewport } from "next";
import { Inter, Lora, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { AppProviders } from "@/providers";
import { WebVitalsReporter } from "@/components/layout/web-vitals-reporter";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
  display: "swap",
});

const lora = Lora({
  variable: "--font-lora",
  subsets: ["latin"],
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains-mono",
  subsets: ["latin"],
  display: "swap",
});

export const metadata: Metadata = {
  title: {
    default: "SRapi — Self-hosted AI gateway",
    template: "%s · SRapi",
  },
  description:
    "One endpoint, every provider, your accounts, your control. Route OpenAI, Anthropic, Gemini and CLI / web-session accounts through a single OpenAI-compatible API with built-in scheduling, quotas and audit logs.",
  applicationName: "SRapi",
  authors: [{ name: "SRapi" }],
  formatDetection: { telephone: false, email: false, address: false },
  robots: { index: false, follow: false },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#F9F6F0" },
    { media: "(prefers-color-scheme: dark)", color: "#111110" },
  ],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${lora.variable} ${jetbrainsMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <body className="min-h-full flex flex-col font-sans bg-srapi-bg text-srapi-text-primary transition-colors duration-300">
        <AppProviders>
          <WebVitalsReporter />
          {children}
        </AppProviders>
      </body>
    </html>
  );
}

