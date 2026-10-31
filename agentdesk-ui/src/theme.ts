import { onMounted, onUnmounted, ref } from "vue";

type Theme = "system" | "dark" | "light";
const key = "agentdesk-theme";

function preference(value: string | null): Theme {
  return value === "dark" || value === "light" ? value : "system";
}

function load(): Theme {
  try {
    return preference(localStorage.getItem(key));
  } catch {
    return "system";
  }
}

export function useTheme() {
  const theme = ref<Theme>(load());
  const device = matchMedia("(prefers-color-scheme: dark)");
  function apply() {
    const dark = theme.value === "dark" || (theme.value === "system" && device.matches);
    document.documentElement.classList.toggle("dark", dark);
    document.documentElement.style.colorScheme = dark ? "dark" : "light";
  }
  function setTheme(value: string) {
    theme.value = preference(value);
    apply();
    try {
      localStorage.setItem(key, theme.value);
    } catch {
      // Keep the current selection even if persistence is unavailable.
    }
  }
  function sync(event: StorageEvent) {
    if (event.key === key || event.key === null) {
      theme.value = load();
      apply();
    }
  }
  apply();
  onMounted(() => {
    device.addEventListener("change", apply);
    window.addEventListener("storage", sync);
  });
  onUnmounted(() => {
    device.removeEventListener("change", apply);
    window.removeEventListener("storage", sync);
  });
  return { theme, setTheme };
}
