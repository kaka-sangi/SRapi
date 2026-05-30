import Landing from "@/features/marketing/landing";

/**
 * Public entry. The marketing landing carries the sign-in form so `/` is both
 * the front door and the login surface (the e2e suite signs in from `/`).
 */
export default function Home() {
  return <Landing />;
}
