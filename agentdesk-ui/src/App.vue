<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { ArrowLeft, ArrowRight, Check, CircleHelp, Inbox, LogOut, RefreshCw } from "@lucide/vue";
import { Button } from "@/components/ui/button";
import { api, ApiError, type History, type Issue, type Session } from "./api";
import { useTheme } from "./theme";
import { useLanguage, formatWaiting, type MessageKey } from "./i18n";

const session = ref<Session | null>(null);
const issues = ref<Issue[]>([]);
const selected = ref<Issue | null>(null);
const history = ref<History[]>([]);
const token = ref("");
const selectedOption = ref("");
const answer = ref("");
const error = ref<MessageKey | "">("");
const notice = ref<MessageKey | "">("");
const busy = ref(false);
const loading = ref(true);
const online = ref(false);
const connectionError = ref(false);
const filter = ref("pending");
const search = ref("");
const { theme, setTheme } = useTheme();
const { language, locale, text, setLanguage } = useLanguage();
let refreshTimer: ReturnType<typeof setInterval> | undefined;
let selectionRequest = 0;

const kinds = computed(() => ({
  decision: text.value.decision,
  action: text.value.action,
  notice: text.value.notice,
}));
const statuses = computed(() => ({
  pending: text.value.pending,
  answered: text.value.answered,
  acked: text.value.acked,
  withdrawn: text.value.withdrawn,
}));
const eventNames = computed<Record<string, string>>(() => ({
  created: text.value.createdEvent,
  updated: text.value.updatedEvent,
  answered: text.value.answeredEvent,
  acked: text.value.ackedEvent,
  withdrawn: text.value.withdrawnEvent,
  deferred: text.value.deferredEvent,
}));
const pendingCount = computed(() => issues.value.filter((i) => i.status === "pending").length);
const nextPending = computed(() =>
  issues.value.find(
    (issue) =>
      issue.status === "pending" && !issue.deferred && issue.issue_id !== selected.value?.issue_id,
  ),
);
const visibleIssues = computed(() =>
  issues.value.filter((i) => {
    const matchesFilter =
      filter.value === "all" ||
      (filter.value === "deferred"
        ? i.deferred && i.status === "pending"
        : i.status === "pending" && !i.deferred);
    return (
      matchesFilter &&
      `${i.title} ${i.issue_id} ${i.context}`.toLowerCase().includes(search.value.toLowerCase())
    );
  }),
);
const canReply = computed(() =>
  selected.value?.kind === "decision" ? !!selectedOption.value || !!answer.value.trim() : true,
);

function date(value: string) {
  return new Date(value).toLocaleString(locale.value, { hour12: false });
}

function waiting(value: string) {
  return formatWaiting(value, locale.value);
}

function handleError(e: unknown) {
  connectionError.value = !(e instanceof ApiError) || e.status >= 500;
  if (connectionError.value) online.value = false;
  if (e instanceof ApiError && e.status === 401) {
    session.value = null;
    issues.value = [];
    selected.value = null;
    history.value = [];
    error.value = "sessionExpired";
  } else if (e instanceof ApiError) {
    const messages: Partial<Record<string, MessageKey>> = {
      schema_version: "versionError",
      not_found: "notFound",
      origin: "forbidden",
      conflict: "conflict",
      lease_conflict: "conflict",
      invalid_request: "invalidRequest",
      invalid_json: "invalidRequest",
      content_type: "invalidRequest",
    };
    error.value = messages[e.code] ?? "serviceError";
  } else {
    error.value = "connectionError";
  }
}

async function refresh() {
  if (!session.value) return;
  try {
    issues.value = await api<Issue[]>("/issues");
    online.value = true;
    if (connectionError.value) {
      error.value = "";
      connectionError.value = false;
    }
  } catch (e) {
    handleError(e);
  }
}

async function selectIssue(id: string) {
  const request = ++selectionRequest;
  error.value = "";
  notice.value = "";
  try {
    const [issue, events] = await Promise.all([
      api<Issue>(`/issues/${encodeURIComponent(id)}`),
      api<History[]>(`/issues/${encodeURIComponent(id)}/history`),
    ]);
    if (request !== selectionRequest) return;
    selected.value = issue;
    history.value = events;
    selectedOption.value = "";
    answer.value = "";
    const url = new URL(location.href);
    url.searchParams.set("issue", id);
    window.history.replaceState(null, "", url);
  } catch (e) {
    handleError(e);
  }
}

function closeIssue() {
  selectionRequest++;
  selected.value = null;
  const url = new URL(location.href);
  url.searchParams.delete("issue");
  window.history.replaceState(null, "", url);
}

async function openNextIssue() {
  const next = nextPending.value;
  if (!next || busy.value) return;
  busy.value = true;
  try {
    await selectIssue(next.issue_id);
    if (selected.value?.issue_id === next.issue_id) {
      await nextTick();
      document.getElementById("issue-title")?.focus({ preventScroll: true });
      window.scrollTo({ top: 0, behavior: "auto" });
    }
  } finally {
    busy.value = false;
  }
}

async function login() {
  busy.value = true;
  error.value = "";
  const credential = token.value.trim();
  token.value = "";
  try {
    session.value = await api<Session>("/session", { token: credential });
    await refresh();
    const issue = new URL(location.href).searchParams.get("issue");
    if (issue) await selectIssue(issue);
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}

async function logout() {
  busy.value = true;
  try {
    await api("/session", undefined, "DELETE");
    session.value = null;
    issues.value = [];
    selected.value = null;
    history.value = [];
    answer.value = "";
    error.value = "";
    notice.value = "";
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}

async function mutate(defer = false) {
  const issue = selected.value;
  if (!issue || busy.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    const path = `/issues/${encodeURIComponent(issue.issue_id)}/${defer ? "defer" : "reply"}`;
    const body = defer
      ? { revision: issue.revision, deferred: !issue.deferred }
      : {
          revision: issue.revision,
          option_id: selectedOption.value,
          text: answer.value,
          ack: issue.kind !== "decision",
        };
    selected.value = await api<Issue>(path, body);
    history.value = await api<History[]>(`/issues/${issue.issue_id}/history`);
    notice.value = defer ? "deferSaved" : "replySaved";
    await refresh();
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      await selectIssue(issue.issue_id);
      error.value = "conflict";
      await refresh();
    } else {
      handleError(e);
    }
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  const fragment = new URLSearchParams(location.hash.slice(1));
  const loginToken = fragment.get("token");
  if (loginToken) {
    // Remove the credential before any API request or user navigation.
    window.history.replaceState(null, "", location.pathname + location.search);
    token.value = loginToken;
    await login();
  } else {
    try {
      session.value = await api<Session>("/session");
      await refresh();
      const id = new URL(location.href).searchParams.get("issue");
      if (id) await selectIssue(id);
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) handleError(e);
    }
  }
  loading.value = false;
  refreshTimer = setInterval(() => {
    if (document.visibilityState === "visible" && !busy.value) void refresh();
  }, 10000);
});
onUnmounted(() => clearInterval(refreshTimer));
</script>

<template>
  <div class="min-h-svh bg-background text-foreground">
    <header class="border-b bg-card">
      <div class="mx-auto flex max-w-7xl flex-wrap items-center gap-3 px-4 py-4 sm:px-8">
        <div
          class="flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground"
        >
          <Inbox class="size-5" aria-hidden="true" />
        </div>
        <div class="min-w-0 flex-1 sm:flex-none">
          <h1 class="font-semibold tracking-tight">Agentdesk</h1>
          <p class="truncate text-xs text-muted-foreground">{{ session?.name ?? text.tagline }}</p>
        </div>
        <div
          v-if="session"
          class="ml-auto flex items-center gap-2 text-xs text-muted-foreground"
          role="status"
          :aria-label="online ? text.connected : text.disconnected"
        >
          <span class="size-2 rounded-full" :class="online ? 'bg-emerald-500' : 'bg-amber-500'" />
          <span class="hidden sm:inline">{{ online ? text.connected : text.disconnected }}</span>
        </div>
        <div
          class="flex w-full items-center justify-end gap-2 sm:w-auto"
          :class="!session && 'sm:ml-auto'"
        >
          <select
            :value="language"
            :aria-label="text.language"
            class="h-9 min-w-0 flex-1 rounded-md border bg-background px-2 text-base sm:w-36 sm:flex-none sm:text-sm"
            @change="setLanguage(($event.target as HTMLSelectElement).value)"
          >
            <option value="system">{{ text.systemLanguage }}</option>
            <option value="en">English</option>
            <option value="zh-TW">繁體中文</option>
          </select>
          <select
            :value="theme"
            :aria-label="text.theme"
            class="h-9 min-w-0 flex-1 rounded-md border bg-background px-2 text-base sm:w-36 sm:flex-none sm:text-sm"
            @change="setTheme(($event.target as HTMLSelectElement).value)"
          >
            <option value="system">{{ text.systemTheme }}</option>
            <option value="dark">{{ text.darkTheme }}</option>
            <option value="light">{{ text.lightTheme }}</option>
          </select>
          <Button
            v-if="session"
            variant="ghost"
            size="icon"
            :aria-label="text.logout"
            :disabled="busy"
            @click="logout"
            ><LogOut aria-hidden="true"
          /></Button>
        </div>
      </div>
    </header>

    <main v-if="loading" class="p-12 text-center text-muted-foreground" role="status">
      {{ text.connecting }}
    </main>

    <main v-else-if="!session" class="mx-auto max-w-md px-6 py-20">
      <div class="rounded-2xl border bg-card p-6 shadow-sm sm:p-8">
        <h2 class="text-2xl font-semibold tracking-tight">{{ text.loginHeading }}</h2>
        <p class="mt-3 text-sm leading-6 text-muted-foreground">
          {{ text.loginHelp }}
        </p>
        <form class="mt-6 space-y-4" @submit.prevent="login">
          <label for="token" class="block text-sm font-medium">{{ text.loginToken }}</label>
          <input
            id="token"
            v-model="token"
            type="password"
            autocomplete="off"
            class="field"
            required
            :disabled="busy"
          />
          <p v-if="error" class="text-sm text-destructive" role="alert">{{ text[error] }}</p>
          <Button type="submit" class="w-full" :disabled="busy || !token.trim()">{{
            busy ? text.signingIn : text.signIn
          }}</Button>
        </form>
      </div>
      <p class="mt-5 text-center text-xs leading-5 text-muted-foreground">
        {{ text.vpnHelp }}
      </p>
    </main>

    <main v-else class="mx-auto max-w-7xl px-4 py-6 sm:px-8 sm:py-8">
      <div
        v-if="error"
        class="mb-5 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive"
        role="alert"
      >
        {{ text[error] }}
      </div>
      <div
        v-if="notice"
        class="mb-5 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-4 text-sm"
        role="status"
      >
        {{ text[notice] }}
      </div>
      <div class="grid grid-cols-1 gap-6 lg:grid-cols-[340px_minmax(0,1fr)]">
        <section
          class="min-w-0 [overflow-wrap:anywhere]"
          :class="selected ? 'hidden lg:block' : ''"
          :aria-label="text.issueList"
        >
          <div class="mb-5 flex items-center justify-between">
            <div>
              <h2 class="text-2xl font-semibold tracking-tight">{{ text.inbox }}</h2>
              <p class="mt-1 text-sm text-muted-foreground">
                {{
                  (pendingCount === 1 ? text.pendingOne : text.pendingMany).replace(
                    "{count}",
                    String(pendingCount),
                  )
                }}
              </p>
            </div>
            <Button variant="outline" size="icon" :aria-label="text.refreshList" @click="refresh"
              ><RefreshCw aria-hidden="true"
            /></Button>
          </div>
          <label for="search" class="sr-only">{{ text.search }}</label>
          <input
            id="search"
            v-model="search"
            type="search"
            :placeholder="text.searchPlaceholder"
            class="field mb-3"
          />
          <div class="mb-5 flex gap-1 rounded-lg bg-muted p-1" :aria-label="text.filter">
            <Button
              v-for="tab in [
                { id: 'pending', label: text.pending },
                { id: 'deferred', label: text.deferred },
                { id: 'all', label: text.all },
              ]"
              :key="tab.id"
              :variant="filter === tab.id ? 'secondary' : 'ghost'"
              size="sm"
              class="flex-1"
              :aria-pressed="filter === tab.id"
              @click="filter = tab.id"
              >{{ tab.label }}</Button
            >
          </div>
          <div class="space-y-2">
            <button
              v-for="issue in visibleIssues"
              :key="issue.issue_id"
              class="w-full rounded-xl border p-4 text-left transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
              :class="
                selected?.issue_id === issue.issue_id ? 'border-primary bg-accent' : 'bg-card'
              "
              @click="selectIssue(issue.issue_id)"
            >
              <div class="flex items-center gap-2 text-xs text-muted-foreground">
                <span class="badge">{{ kinds[issue.kind] }}</span
                ><span
                  v-if="issue.priority === 'high'"
                  class="text-amber-600 dark:text-amber-400"
                  >{{ text.highPriority }}</span
                ><span class="ml-auto">{{ waiting(issue.created_at) }}</span>
              </div>
              <h3 class="mt-3 break-words font-medium leading-6">{{ issue.title }}</h3>
              <p class="mt-2 truncate text-xs text-muted-foreground">
                {{ issue.blocking_scope || issue.issue_id }}
              </p>
              <p v-if="issue.status !== 'pending'" class="mt-2 text-xs text-muted-foreground">
                {{ statuses[issue.status] }}
              </p>
            </button>
            <div
              v-if="!visibleIssues.length"
              class="rounded-xl border border-dashed px-5 py-12 text-center"
            >
              <Check class="mx-auto mb-3 size-6 text-muted-foreground" aria-hidden="true" />
              <p class="text-sm font-medium">
                {{ search ? text.noMatches : text.noIssues }}
              </p>
              <p class="mt-2 text-xs leading-5 text-muted-foreground">
                {{ text.emptyHelp }}
              </p>
            </div>
          </div>
        </section>

        <section
          v-if="selected"
          class="min-w-0 rounded-2xl border bg-card [overflow-wrap:anywhere]"
          :aria-label="text.issueDetails"
        >
          <div class="border-b p-5 sm:p-7">
            <Button variant="ghost" size="sm" class="-ml-2 mb-4 lg:hidden" @click="closeIssue"
              ><ArrowLeft aria-hidden="true" />{{ text.back }}</Button
            >
            <div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span class="badge">{{ kinds[selected.kind] }}</span
              ><span>{{ selected.issue_id }}</span
              ><span>v{{ selected.revision }}</span
              ><span class="ml-auto">{{ statuses[selected.status] }}</span>
            </div>
            <h2
              id="issue-title"
              tabindex="-1"
              class="mt-4 break-words text-2xl font-semibold leading-snug tracking-tight"
            >
              {{ selected.title }}
            </h2>
            <p class="mt-3 text-xs text-muted-foreground">
              {{ selected.created_by }} · {{ date(selected.created_at) }}
            </p>
          </div>
          <div class="space-y-6 p-5 sm:p-7">
            <section v-if="selected.context">
              <h3 class="section-label">{{ text.background }}</h3>
              <p class="mt-2 whitespace-pre-wrap break-words text-sm leading-7">
                {{ selected.context }}
              </p>
            </section>
            <div v-if="selected.blocking_scope" class="rounded-lg bg-muted p-4">
              <h3 class="section-label">{{ text.blockingScope }}</h3>
              <p class="mt-2 whitespace-pre-wrap break-words text-sm">
                {{ selected.blocking_scope }}
              </p>
            </div>
            <dl class="grid grid-cols-2 gap-3 text-xs">
              <div>
                <dt class="text-muted-foreground">{{ text.coordinator }}</dt>
                <dd class="mt-1 break-all">{{ selected.owner_coordinator }}</dd>
              </div>
              <div v-if="selected.task_ref">
                <dt class="text-muted-foreground">{{ text.task }}</dt>
                <dd class="mt-1 break-all">{{ selected.task_ref }}</dd>
              </div>
              <div v-if="selected.branch">
                <dt class="text-muted-foreground">{{ text.branch }}</dt>
                <dd class="mt-1 break-all">{{ selected.branch }}</dd>
              </div>
              <div v-if="selected.origin_session">
                <dt class="text-muted-foreground">{{ text.originSession }}</dt>
                <dd class="mt-1 break-all">{{ selected.origin_session }}</dd>
              </div>
            </dl>
            <section v-if="selected.related_documents?.length">
              <h3 class="section-label">{{ text.documents }}</h3>
              <ul class="mt-2 space-y-1 text-sm">
                <li v-for="doc in selected.related_documents" :key="doc" class="break-all">
                  {{ doc }}
                </li>
              </ul>
            </section>

            <form
              v-if="selected.status === 'pending'"
              class="space-y-5 border-t pt-6"
              @submit.prevent="mutate()"
            >
              <fieldset
                v-if="selected.kind === 'decision' && selected.options?.length"
                :disabled="busy"
                class="min-w-0 space-y-3"
              >
                <legend class="section-label mb-3">{{ text.chooseOption }}</legend>
                <label
                  v-for="option in selected.options"
                  :key="option.id"
                  class="flex cursor-pointer gap-3 rounded-xl border p-4"
                  :class="selectedOption === option.id ? 'border-primary bg-accent' : ''"
                >
                  <input
                    v-model="selectedOption"
                    type="radio"
                    name="option"
                    :value="option.id"
                    class="mt-1 size-4 shrink-0 accent-primary"
                  />
                  <span class="min-w-0 space-y-2 text-sm"
                    ><span class="block font-medium"
                      >{{ option.label }}
                      <span v-if="option.id === selected.recommended_option" class="badge ml-1">{{
                        text.recommended
                      }}</span></span
                    ><span
                      v-if="option.description"
                      class="block whitespace-pre-wrap leading-6 text-muted-foreground"
                      >{{ option.description }}</span
                    ><span
                      v-if="option.pros"
                      class="block whitespace-pre-wrap text-muted-foreground"
                      >{{ text.pros }}{{ option.pros }}</span
                    ><span
                      v-if="option.cons"
                      class="block whitespace-pre-wrap text-muted-foreground"
                      >{{ text.cons }}{{ option.cons }}</span
                    ></span
                  >
                </label>
                <Button
                  v-if="selectedOption"
                  type="button"
                  variant="ghost"
                  size="sm"
                  class="h-auto max-w-full whitespace-normal text-left"
                  @click="selectedOption = ''"
                  >{{ text.clearSelection }}</Button
                >
              </fieldset>
              <p
                v-if="selected.recommendation_reason"
                class="rounded-lg bg-muted p-4 text-sm leading-6"
              >
                <strong class="font-medium">{{ text.recommendationReason }}</strong
                >{{ selected.recommendation_reason }}
              </p>
              <div>
                <label for="answer" class="section-label">{{
                  selected.kind === "decision" ? text.answer : text.note
                }}</label
                ><textarea
                  id="answer"
                  v-model="answer"
                  rows="4"
                  class="field mt-2 resize-y"
                  :disabled="busy"
                  :placeholder="
                    selected.kind === 'decision' ? text.answerPlaceholder : text.notePlaceholder
                  "
                />
              </div>
              <p
                v-if="selected.kind !== 'decision'"
                class="text-xs leading-5 text-muted-foreground"
              >
                {{ text.ackHelp }}
              </p>
              <div class="flex flex-wrap gap-3">
                <Button type="submit" :disabled="busy || !canReply">{{
                  busy ? text.saving : selected.kind === "decision" ? text.submit : text.acknowledge
                }}</Button
                ><Button type="button" variant="outline" :disabled="busy" @click="mutate(true)">{{
                  selected.deferred ? text.restore : text.defer
                }}</Button>
              </div>
            </form>

            <section
              v-if="selected.reply"
              class="rounded-xl border border-emerald-500/30 bg-emerald-500/5 p-5"
            >
              <h3 class="font-medium">
                {{ selected.reply.ack ? text.acked : text.answerSaved }}
              </h3>
              <p v-if="selected.reply.option_id" class="mt-3 text-sm">
                {{ text.selectedOption
                }}{{
                  selected.options?.find((o) => o.id === selected!.reply!.option_id)?.label ??
                  selected.reply.option_id
                }}
              </p>
              <p
                v-if="selected.reply.text"
                class="mt-2 whitespace-pre-wrap break-words text-sm leading-7"
              >
                {{ selected.reply.text }}
              </p>
              <p class="mt-3 text-xs text-muted-foreground">
                {{ date(selected.reply.at) }} · v{{ selected.reply.revision }}
              </p>
              <p class="mt-3 text-sm">
                {{
                  history.some(
                    (e) =>
                      (e.kind === "answered" || e.kind === "acked") &&
                      e.receipts.some((r) => r.consumer === selected!.owner_coordinator),
                  )
                    ? text.handedOff
                    : text.awaitingHandoff
                }}
              </p>
            </section>

            <div v-if="selected.reply" class="space-y-3">
              <Button
                v-if="nextPending"
                type="button"
                class="h-auto min-h-11 w-full whitespace-normal sm:w-auto"
                :disabled="busy || !online"
                @click="openNextIssue"
                >{{ text.nextPending }}<ArrowRight aria-hidden="true"
              /></Button>
              <p v-else class="text-sm text-muted-foreground">{{ text.noMorePending }}</p>
              <Button v-if="!nextPending" type="button" variant="outline" @click="closeIssue">{{
                text.backToInbox
              }}</Button>
            </div>

            <section class="border-t pt-6">
              <div class="flex items-center justify-between">
                <h3 class="section-label">{{ text.history }}</h3>
                <Button
                  variant="ghost"
                  size="sm"
                  :disabled="busy"
                  @click="selectIssue(selected.issue_id)"
                  ><RefreshCw aria-hidden="true" />{{ text.refreshDetails }}</Button
                >
              </div>
              <ol class="mt-4 space-y-4">
                <li v-for="event in history" :key="event.event_id" class="border-l-2 pl-4">
                  <p class="text-sm">
                    {{ eventNames[event.kind] ?? event.kind }}
                    <span class="text-muted-foreground">· v{{ event.revision }}</span>
                  </p>
                  <p class="mt-1 break-all text-xs text-muted-foreground">
                    {{ date(event.at) }} · {{ event.actor }}
                  </p>
                  <p
                    v-for="receipt in event.receipts"
                    :key="receipt.consumer"
                    class="mt-2 break-all text-xs text-emerald-700 dark:text-emerald-400"
                  >
                    {{ receipt.consumer }} {{ text.received }} · {{ date(receipt.at) }} ·
                    {{ receipt.holder }}
                  </p>
                </li>
              </ol>
            </section>
          </div>
        </section>
        <section
          v-else
          class="hidden min-h-[540px] flex-col items-center justify-center rounded-2xl border border-dashed p-8 text-center lg:flex"
        >
          <CircleHelp class="mb-4 size-10 text-muted-foreground" aria-hidden="true" />
          <h2 class="font-medium">{{ text.welcomeHeading }}</h2>
          <p class="mt-3 max-w-sm text-sm leading-6 text-muted-foreground">
            {{ text.welcomeHelp }}
          </p>
        </section>
      </div>
    </main>
  </div>
</template>
