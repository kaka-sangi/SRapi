import { describe, it, expect, beforeEach } from "vitest";
import {
  SESSION_PRESENT_COOKIE,
  SESSION_ROLE_COOKIE,
  setSessionPresenceCookie,
  clearSessionPresenceCookie,
} from "@/lib/session-cookie";

function readCookie(name: string): string {
  const match = document.cookie.split("; ").find((row) => row.startsWith(`${name}=`));
  return match ? match.slice(name.length + 1) : "";
}

describe("session-cookie", () => {
  beforeEach(() => {
    document.cookie = `${SESSION_PRESENT_COOKIE}=; Path=/; Max-Age=0`;
    document.cookie = `${SESSION_ROLE_COOKIE}=; Path=/; Max-Age=0`;
  });

  it("sets presence and role cookies", () => {
    setSessionPresenceCookie("admin");
    expect(readCookie(SESSION_PRESENT_COOKIE)).toBe("1");
    expect(readCookie(SESSION_ROLE_COOKIE)).toBe("admin");
  });

  it("clears the cookies", () => {
    setSessionPresenceCookie("user");
    clearSessionPresenceCookie();
    // Real browsers and happy-dom may keep the key with an empty value or
    // drop it entirely after Max-Age=0; either outcome is "logged out".
    expect(readCookie(SESSION_PRESENT_COOKIE)).toBe("");
    expect(readCookie(SESSION_ROLE_COOKIE)).toBe("");
  });
});
