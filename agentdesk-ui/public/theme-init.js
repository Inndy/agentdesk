// Run before styles and app rendering, including on the login page.
(() => {
  let theme = "system";
  try {
    const saved = localStorage.getItem("agentdesk-theme");
    if (saved === "light" || saved === "dark") theme = saved;
  } catch {
    // Storage may be unavailable; device preference still works.
  }
  const dark =
    theme === "dark" || (theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.style.colorScheme = dark ? "dark" : "light";
})();
