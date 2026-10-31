import { describe, expect, it } from "vite-plus/test";
import { formatWaiting, resolveLocale } from "./i18n";

describe("language negotiation", () => {
  it("honors an explicit choice over browser preferences", () => {
    expect(resolveLocale("en", ["zh-TW"])).toBe("en");
    expect(resolveLocale("zh-TW", ["en-US"])).toBe("zh-TW");
  });
  it("uses the first supported browser language and falls back to English", () => {
    expect(resolveLocale("system", ["fr-FR", "zh-Hant-HK", "en"])).toBe("zh-TW");
    expect(resolveLocale(null, ["en-GB", "zh-TW"])).toBe("en");
    expect(resolveLocale("invalid", ["ZH-tw"])).toBe("zh-TW");
    expect(resolveLocale(null, ["fr-FR"])).toBe("en");
    expect(resolveLocale(null, [])).toBe("en");
  });
});

describe("waiting time", () => {
  const now = Date.parse("2026-10-31T12:00:00Z");
  const ago = (minutes: number) => new Date(now - minutes * 60000).toISOString();
  it("formats singular and plural units at minute, hour, and day boundaries", () => {
    expect(formatWaiting(ago(1), "en", now)).toBe("1 minute");
    expect(formatWaiting(ago(59), "en", now)).toBe("59 minutes");
    expect(formatWaiting(ago(60), "en", now)).toBe("1 hour");
    expect(formatWaiting(ago(120), "en", now)).toBe("2 hours");
    expect(formatWaiting(ago(1440), "en", now)).toBe("1 day");
    expect(formatWaiting(ago(-1), "en", now)).toBe("0 minutes");
  });
  it("uses Traditional Chinese units", () => {
    expect(formatWaiting(ago(2), "zh-TW", now)).toBe("2 分鐘");
    expect(formatWaiting(ago(60), "zh-TW", now)).toBe("1 小時");
  });
});
