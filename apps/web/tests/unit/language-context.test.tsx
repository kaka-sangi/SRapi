import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LanguageProvider, useLanguage } from "@/context/LanguageContext";

function Probe() {
  const { language, t, toggleLanguage } = useLanguage();
  return (
    <div>
      <span data-testid="lang">{language}</span>
      <span data-testid="title">{t("verifyOperator")}</span>
      <button type="button" onClick={toggleLanguage}>
        toggle
      </button>
    </div>
  );
}

describe("LanguageProvider", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("starts in English and renders the v0.1.0 sign-in title", () => {
    render(
      <LanguageProvider>
        <Probe />
      </LanguageProvider>,
    );

    expect(screen.getByTestId("lang")).toHaveTextContent("en");
    expect(screen.getByTestId("title")).toHaveTextContent("Sign in to SRapi");
  });

  it("toggles to Chinese and persists", async () => {
    render(
      <LanguageProvider>
        <Probe />
      </LanguageProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: "toggle" }));

    expect(screen.getByTestId("lang")).toHaveTextContent("zh");
    expect(screen.getByTestId("title")).toHaveTextContent("登录 SRapi");
    await waitFor(() => expect(window.localStorage.getItem("srapi_lang")).toBe("zh"));
  });

  it("falls back to the key when no translation exists", () => {
    function Fallback() {
      const { t } = useLanguage();
      return <span data-testid="missing">{t("definitelyMissingKey")}</span>;
    }
    render(
      <LanguageProvider>
        <Fallback />
      </LanguageProvider>,
    );
    expect(screen.getByTestId("missing")).toHaveTextContent("definitelyMissingKey");
  });

  it("reads persisted language on remount", async () => {
    window.localStorage.setItem("srapi_lang", "zh");
    render(
      <LanguageProvider>
        <Probe />
      </LanguageProvider>,
    );
    // useEffect runs after first paint; flush.
    await act(async () => {});
    expect(screen.getByTestId("lang")).toHaveTextContent("zh");
  });
});
