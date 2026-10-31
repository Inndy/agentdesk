import { computed, onMounted, onUnmounted, ref, watchEffect } from "vue";

export const en = {
  tagline: "Human issue inbox",
  connected: "Connected",
  disconnected: "Disconnected",
  language: "Language",
  systemLanguage: "Browser default",
  theme: "Appearance",
  systemTheme: "System theme",
  darkTheme: "Dark mode",
  lightTheme: "Light mode",
  logout: "Sign out",
  connecting: "Connecting…",
  loginHeading: "Sign in to your inbox",
  loginHelp:
    "Run agentdesk login on your computer and open the generated URL, or enter your human login token below.",
  loginToken: "Login token",
  signingIn: "Signing in…",
  signIn: "Sign in",
  vpnHelp:
    "On your phone, connect to your VPN first, then open the private-network login URL generated on your computer.",
  issueList: "Issue list",
  inbox: "Inbox",
  pendingOne: "1 issue awaiting your reply",
  pendingMany: "{count} issues awaiting your reply",
  refreshList: "Refresh list",
  search: "Search issues",
  searchPlaceholder: "Search titles, content, or IDs",
  filter: "Filter issues",
  pending: "Pending",
  deferred: "Deferred",
  all: "All",
  decision: "Decision needed",
  action: "Action needed",
  notice: "Notice",
  answered: "Answered",
  acked: "Acknowledged",
  withdrawn: "Withdrawn",
  highPriority: "High priority",
  noMatches: "No matching issues",
  noIssues: "No issues in this view",
  emptyHelp: "Issues created by your agents will appear here automatically.",
  issueDetails: "Issue details",
  back: "Back to list",
  background: "Background",
  blockingScope: "Impact and blocking scope",
  coordinator: "Coordinator",
  task: "Task",
  branch: "Branch",
  originSession: "Origin session",
  documents: "Related documents",
  chooseOption: "Choose an option, or write your own answer below",
  recommended: "Agent recommendation",
  pros: "Benefits: ",
  cons: "Tradeoffs: ",
  clearSelection: "Clear selection and write your own answer",
  recommendationReason: "Why this is recommended: ",
  answer: "Your answer or additional context",
  note: "Note (optional)",
  answerPlaceholder: "You can propose your own approach…",
  notePlaceholder: "Add anything the agent should know…",
  ackHelp:
    "Acknowledging only confirms that you received this item. It does not mean the requested action is complete.",
  saving: "Saving…",
  submit: "Submit answer",
  acknowledge: "Acknowledge",
  restore: "Move back to pending",
  defer: "Defer",
  answerSaved: "Your answer has been saved",
  nextPending: "Next pending issue",
  backToInbox: "Back to inbox",
  noMorePending: "No other issues are awaiting your reply. Deferred issues are saved for later.",
  selectedOption: "Selected: ",
  handedOff: "The coordinator has picked this up. Track further progress in the project.",
  awaitingHandoff: "Waiting for the coordinator to pick this up.",
  history: "History and handoff status",
  refreshDetails: "Refresh details",
  createdEvent: "Issue created",
  updatedEvent: "Issue updated",
  answeredEvent: "Human answered",
  ackedEvent: "Human acknowledged",
  withdrawnEvent: "Issue withdrawn",
  deferredEvent: "Deferral updated",
  received: "Picked up",
  welcomeHeading: "Keep the decisions that need you here.",
  welcomeHelp:
    "Select an issue to review its background, answer a question, or acknowledge a notice. Your agents can keep working on other tasks.",
  sessionExpired:
    "Your login is invalid or has expired. Reopen a login URL or enter your token again.",
  connectionError: "Cannot connect. Check the local service and your VPN.",
  serviceError: "The service could not complete the request. Please try again.",
  versionError: "This service version is not supported. Update Agentdesk and reload the page.",
  notFound: "This issue could not be found. Refresh the list and try again.",
  invalidRequest: "The request could not be accepted. Refresh the issue and check your input.",
  forbidden: "This request is not allowed. Open Agentdesk directly from its login URL.",
  conflict:
    "The issue version or status has changed. The latest content has been loaded; review it before submitting again.",
  deferSaved: "Deferral updated.",
  replySaved: "Your reply has been saved and is waiting for the coordinator.",
};

export type MessageKey = keyof typeof en;
const zh: Record<MessageKey, string> = {
  tagline: "人類議題收件匣",
  connected: "已連線",
  disconnected: "連線中斷",
  language: "語言",
  systemLanguage: "跟隨瀏覽器",
  theme: "顯示模式",
  systemTheme: "跟隨裝置",
  darkTheme: "深色模式",
  lightTheme: "淺色模式",
  logout: "登出",
  connecting: "正在連線…",
  loginHeading: "登入收件匣",
  loginHelp: "在電腦執行 agentdesk login，開啟產生的登入網址，或在下方輸入人類登入 token。",
  loginToken: "登入 token",
  signingIn: "登入中…",
  signIn: "登入",
  vpnHelp: "手機請先連上 VPN，再開啟電腦產生的內網登入網址。",
  issueList: "議題清單",
  inbox: "收件匣",
  pendingOne: "1 個議題等待你回覆",
  pendingMany: "{count} 個議題等待你回覆",
  refreshList: "重新整理清單",
  search: "搜尋議題",
  searchPlaceholder: "搜尋標題、內容或 ID",
  filter: "清單篩選",
  pending: "待回覆",
  deferred: "稍後處理",
  all: "全部",
  decision: "需要決議",
  action: "需要處理",
  notice: "通知",
  answered: "已回答",
  acked: "已確認收到",
  withdrawn: "已撤銷",
  highPriority: "高優先度",
  noMatches: "沒有符合的議題",
  noIssues: "目前沒有這類議題",
  emptyHelp: "Agent 建立的議題會自動出現在這裡。",
  issueDetails: "議題詳情",
  back: "返回清單",
  background: "背景",
  blockingScope: "影響與阻擋範圍",
  coordinator: "接手者",
  task: "工作項目",
  branch: "分支",
  originSession: "來源 session",
  documents: "相關文件",
  chooseOption: "選擇方案，或在下方自由回答",
  recommended: "Agent 建議",
  pros: "優點：",
  cons: "代價：",
  clearSelection: "清除選擇，改用自由回答",
  recommendationReason: "推薦理由：",
  answer: "你的回答或補充說明",
  note: "附註（選填）",
  answerPlaceholder: "也可以提出自己的方案…",
  notePlaceholder: "補充需要讓 agent 知道的資訊…",
  ackHelp: "「確認收到」只表示你已收到此事項，不代表外部操作已完成。",
  saving: "保存中…",
  submit: "送出回答",
  acknowledge: "確認收到",
  restore: "移回待回覆",
  defer: "稍後處理",
  answerSaved: "已保存你的回答",
  nextPending: "下一個待回覆議題",
  backToInbox: "回到收件匣",
  noMorePending: "目前沒有其他待回覆議題，稍後處理的議題仍保留在原清單。",
  selectedOption: "選擇：",
  handedOff: "Coordinator 已接手；後續工作完成與否由專案追蹤。",
  awaitingHandoff: "等待 coordinator 接手。",
  history: "歷史與交付狀態",
  refreshDetails: "更新詳情",
  createdEvent: "建立議題",
  updatedEvent: "修改議題",
  answeredEvent: "人類已回答",
  ackedEvent: "人類已確認收到",
  withdrawnEvent: "撤銷議題",
  deferredEvent: "調整稍後處理標記",
  received: "已接手",
  welcomeHeading: "把需要你決定的事，留在這裡。",
  welcomeHelp: "從左側選擇議題，查看背景、回答問題，或確認收到通知。Agent 可以同時繼續其他工作。",
  sessionExpired: "登入已失效，請重新開啟登入網址或輸入 token。",
  connectionError: "無法連線，請確認本機服務與 VPN。",
  serviceError: "服務無法完成請求，請稍後再試。",
  versionError: "不支援的服務版本，請更新 Agentdesk 並重新整理頁面。",
  notFound: "找不到此議題，請重新整理清單後再試。",
  invalidRequest: "無法接受此請求，請更新議題並檢查輸入內容。",
  forbidden: "不允許此請求，請直接從登入網址開啟 Agentdesk。",
  conflict: "議題版本或狀態已改變。已載入最新內容，請重新確認後再送出。",
  deferSaved: "已更新稍後處理標記。",
  replySaved: "回覆已保存，等待 coordinator 接手。",
};

export type Locale = "en" | "zh-TW";
type Language = Locale | "system";
const key = "agentdesk-language";

export function resolveLocale(preference: string | null, languages: readonly string[]): Locale {
  if (preference === "en" || preference === "zh-TW") return preference;
  const supported = languages.find((language) => /^(en|zh)(-|$)/i.test(language));
  return supported?.toLowerCase().startsWith("zh") ? "zh-TW" : "en";
}

export function formatWaiting(value: string, locale: Locale, now = Date.now()) {
  const minutes = Math.max(0, Math.floor((now - new Date(value).getTime()) / 60000));
  const unit = minutes < 60 ? "minute" : minutes < 1440 ? "hour" : "day";
  const amount = unit === "minute" ? minutes : Math.floor(minutes / (unit === "hour" ? 60 : 1440));
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "long" }).format(amount);
}

function load(): Language {
  try {
    const value = localStorage.getItem(key);
    return value === "en" || value === "zh-TW" ? value : "system";
  } catch {
    return "system";
  }
}

export function useLanguage() {
  const language = ref<Language>(load());
  const browserLanguages = ref([...navigator.languages]);
  const locale = computed(() => resolveLocale(language.value, browserLanguages.value));
  const text = computed<Record<MessageKey, string>>(() => (locale.value === "en" ? en : zh));
  watchEffect(() => {
    document.documentElement.lang = locale.value;
  });
  function setLanguage(value: string) {
    language.value = value === "en" || value === "zh-TW" ? value : "system";
    try {
      localStorage.setItem(key, language.value);
    } catch {
      /* Keep the in-memory choice. */
    }
  }
  function sync(event: StorageEvent) {
    if (event.key === key || event.key === null) language.value = load();
  }
  function syncBrowser() {
    browserLanguages.value = [...navigator.languages];
  }
  onMounted(() => {
    window.addEventListener("storage", sync);
    window.addEventListener("languagechange", syncBrowser);
  });
  onUnmounted(() => {
    window.removeEventListener("storage", sync);
    window.removeEventListener("languagechange", syncBrowser);
  });
  return { language, locale, text, setLanguage };
}
