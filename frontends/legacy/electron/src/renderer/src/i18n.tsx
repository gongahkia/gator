import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { SettingsSnapshot } from "@shared/ipc/contracts";

export type SupportedLocale = Exclude<SettingsSnapshot["appLanguage"], "system">;

type TranslationKey =
  | "action.apply"
  | "action.calendar"
  | "action.commandPalette"
  | "action.diagnostics"
  | "action.moveDown"
  | "action.moveUp"
  | "action.notifications"
  | "action.refresh"
  | "action.reset"
  | "action.settings"
  | "action.splitView"
  | "action.viewDiagnostics"
  | "app.name"
  | "command.close"
  | "command.empty.noCommands"
  | "command.empty.noLocalResults"
  | "command.empty.trySection"
  | "command.empty.unmatched"
  | "command.filter"
  | "command.loading.detail"
  | "command.loading.title"
  | "command.placeholder"
  | "command.title"
  | "diagnostics.description"
  | "diagnostics.includeGooglePayloads"
  | "diagnostics.includeGooglePayloads.description"
  | "diagnostics.includePerformance"
  | "diagnostics.includePerformance.description"
  | "diagnostics.title"
  | "hotkeys.calendar"
  | "hotkeys.navigation"
  | "language.description"
  | "language.english"
  | "language.japanese"
  | "language.korean"
  | "language.malay"
  | "language.simplifiedChinese"
  | "language.system"
  | "language.tamil"
  | "layout.calendarViewModes"
  | "layout.navigationPlacement"
  | "layout.navigationPlacement.description"
  | "layout.navigationTabs"
  | "layout.toolbarActions"
  | "nav.calendar"
  | "nav.calendars"
  | "nav.hideSidebar"
  | "nav.notes"
  | "nav.showAllCalendars"
  | "nav.showSidebar"
  | "nav.tasks"
  | "settings.agentAccess"
  | "settings.appLanguage"
  | "settings.general"
  | "settings.layout"
  | "settings.language"
  | "settings.startup"
  | "settings.sync";

type Dictionary = Record<TranslationKey, string>;

const en: Dictionary = {
  "action.apply": "Apply",
  "action.calendar": "Calendar",
  "action.commandPalette": "Command palette",
  "action.diagnostics": "Diagnostics",
  "action.moveDown": "Move down",
  "action.moveUp": "Move up",
  "action.notifications": "Notifications",
  "action.refresh": "Reload",
  "action.reset": "Reset",
  "action.settings": "Settings",
  "action.splitView": "Split view",
  "action.viewDiagnostics": "View diagnostics",
  "app.name": "Hot Cross Buns",
  "command.close": "Close command palette",
  "command.empty.noCommands": "No commands found",
  "command.empty.noLocalResults": "No local results",
  "command.empty.trySection": "Try a section name or action.",
  "command.empty.unmatched": "No commands or cached items matched this query.",
  "command.filter": "Filter commands",
  "command.loading.detail": "Tasks, events, notes, birthdays, and calendar text.",
  "command.loading.title": "Searching planner data",
  "command.placeholder": "Run a command or search planner data",
  "command.title": "Command palette",
  "diagnostics.description": "Inspect logs, mutation history, sync queues, and support bundles.",
  "diagnostics.includeGooglePayloads": "Include field-redacted Google payloads in local logs",
  "diagnostics.includeGooglePayloads.description": "Future local logs may include field-redacted Google troubleshooting snippets.",
  "diagnostics.includePerformance": "Include performance diagnostics",
  "diagnostics.includePerformance.description": "Includes startup, migration, slow query, and MCP request timings in diagnostics.",
  "diagnostics.title": "Diagnostics",
  "hotkeys.calendar": "Calendar",
  "hotkeys.navigation": "Navigation",
  "language.description": "System Default follows your macOS language order.",
  "language.english": "English",
  "language.japanese": "Japanese",
  "language.korean": "Korean",
  "language.malay": "Bahasa Melayu",
  "language.simplifiedChinese": "Mandarin Chinese (Simplified)",
  "language.system": "System Default",
  "language.tamil": "Tamil",
  "layout.calendarViewModes": "Calendar view modes",
  "layout.navigationPlacement": "Navigation placement",
  "layout.navigationPlacement.description": "Left keeps the native sidebar placement; right moves the same tabs around the current view.",
  "layout.navigationTabs": "Navigation tabs",
  "layout.toolbarActions": "Toolbar actions",
  "nav.calendar": "Calendar",
  "nav.calendars": "Calendars",
  "nav.hideSidebar": "Hide sidebar",
  "nav.notes": "Notes",
  "nav.showAllCalendars": "Show all calendars",
  "nav.showSidebar": "Show sidebar",
  "nav.tasks": "Tasks",
  "settings.agentAccess": "Agent access",
  "settings.appLanguage": "App language",
  "settings.general": "General",
  "settings.language": "Language",
  "settings.layout": "Layout",
  "settings.startup": "Startup",
  "settings.sync": "Sync"
};

const dictionaries: Record<SupportedLocale, Dictionary> = {
  en,
  "zh-Hans": {
    ...en,
    "action.apply": "应用",
    "action.calendar": "日历",
    "action.commandPalette": "命令面板",
    "action.diagnostics": "诊断",
    "action.moveDown": "下移",
    "action.moveUp": "上移",
    "action.notifications": "通知",
    "action.refresh": "重新加载",
    "action.reset": "重置",
    "action.settings": "设置",
    "action.splitView": "分屏视图",
    "action.viewDiagnostics": "查看诊断",
    "command.close": "关闭命令面板",
    "command.empty.noCommands": "未找到命令",
    "command.empty.noLocalResults": "没有本地结果",
    "command.empty.trySection": "尝试输入分区名称或操作。",
    "command.empty.unmatched": "没有命令或缓存项目匹配此查询。",
    "command.filter": "筛选命令",
    "command.loading.detail": "任务、事件、笔记、生日和日历文本。",
    "command.loading.title": "正在搜索规划数据",
    "command.placeholder": "运行命令或搜索规划数据",
    "command.title": "命令面板",
    "diagnostics.description": "查看日志、变更历史、同步队列和支持包。",
    "diagnostics.includeGooglePayloads": "在本地日志中包含字段已脱敏的 Google 载荷",
    "diagnostics.includeGooglePayloads.description": "未来的本地日志可能包含字段已脱敏的 Google 排错片段。",
    "diagnostics.includePerformance": "包含性能诊断",
    "diagnostics.includePerformance.description": "包含启动、迁移、慢查询和 MCP 请求耗时。",
    "diagnostics.title": "诊断",
    "language.description": "系统默认会跟随 macOS 语言顺序。",
    "language.english": "英语",
    "language.japanese": "日语",
    "language.korean": "韩语",
    "language.malay": "马来语",
    "language.simplifiedChinese": "普通话（简体中文）",
    "language.system": "系统默认",
    "language.tamil": "泰米尔语",
    "layout.calendarViewModes": "日历视图模式",
    "layout.navigationPlacement": "导航位置",
    "layout.navigationPlacement.description": "左侧保留原生侧边栏位置；右侧将相同标签移到当前视图旁。",
    "layout.navigationTabs": "导航标签",
    "layout.toolbarActions": "工具栏操作",
    "nav.calendar": "日历",
    "nav.calendars": "日历",
    "nav.hideSidebar": "隐藏侧边栏",
    "nav.notes": "笔记",
    "nav.showAllCalendars": "显示所有日历",
    "nav.showSidebar": "显示侧边栏",
    "nav.tasks": "任务",
    "settings.agentAccess": "代理访问",
    "settings.appLanguage": "应用语言",
    "settings.general": "通用",
    "settings.language": "语言",
    "settings.layout": "布局",
    "settings.startup": "启动",
    "settings.sync": "同步"
  },
  ta: {
    ...en,
    "action.apply": "செயல்படுத்து",
    "action.calendar": "நாட்காட்டி",
    "action.commandPalette": "கட்டளை பலகை",
    "action.diagnostics": "கண்டறிதல்",
    "action.moveDown": "கீழே நகர்த்து",
    "action.moveUp": "மேலே நகர்த்து",
    "action.notifications": "அறிவிப்புகள்",
    "action.refresh": "மீண்டும் ஏற்று",
    "action.reset": "மீட்டமை",
    "action.settings": "அமைப்புகள்",
    "action.splitView": "பிரிப்பு காட்சி",
    "action.viewDiagnostics": "கண்டறிதலை பார்க்க",
    "command.close": "கட்டளை பலகையை மூடு",
    "command.empty.noCommands": "கட்டளைகள் இல்லை",
    "command.empty.noLocalResults": "உள்ளூர் முடிவுகள் இல்லை",
    "command.empty.trySection": "பிரிவு பெயர் அல்லது செயலை முயற்சிக்கவும்.",
    "command.empty.unmatched": "இந்த தேடலுக்கு கட்டளைகள் அல்லது சேமித்த உருப்படிகள் இல்லை.",
    "command.filter": "கட்டளைகளை வடிகட்டு",
    "command.loading.detail": "பணிகள், நிகழ்வுகள், குறிப்புகள், பிறந்தநாள்கள் மற்றும் நாட்காட்டி உரை.",
    "command.loading.title": "திட்ட தரவு தேடப்படுகிறது",
    "command.placeholder": "கட்டளையை இயக்கவும் அல்லது திட்ட தரவை தேடவும்",
    "command.title": "கட்டளை பலகை",
    "diagnostics.description": "பதிவுகள், மாற்ற வரலாறு, ஒத்திசைவு வரிசைகள் மற்றும் ஆதரவு தொகுப்புகளை ஆய்வு செய்.",
    "diagnostics.includeGooglePayloads": "உள்ளூர் பதிவுகளில் புலம் மறைக்கப்பட்ட Google payloads சேர்க்கவும்",
    "diagnostics.includeGooglePayloads.description": "வருங்கால உள்ளூர் பதிவுகளில் புலம் மறைக்கப்பட்ட Google troubleshooting துணுக்குகள் இருக்கலாம்.",
    "diagnostics.includePerformance": "செயல்திறன் கண்டறிதலை சேர்க்கவும்",
    "diagnostics.includePerformance.description": "தொடக்கம், மாற்றம், மெதுவான query, MCP request நேரங்களை சேர்க்கிறது.",
    "diagnostics.title": "கண்டறிதல்",
    "language.description": "System Default உங்கள் macOS மொழி வரிசையைப் பின்பற்றும்.",
    "language.english": "ஆங்கிலம்",
    "language.japanese": "ஜப்பானியம்",
    "language.korean": "கொரியம்",
    "language.malay": "மலாய்",
    "language.simplifiedChinese": "மந்தரின் சீனம் (எளிய)",
    "language.system": "System Default",
    "language.tamil": "தமிழ்",
    "layout.calendarViewModes": "நாட்காட்டி காட்சி முறைகள்",
    "layout.navigationPlacement": "வழிசெலுத்தல் இடம்",
    "layout.navigationPlacement.description": "இடது பக்கம் native sidebar இடத்தை வைத்திருக்கும்; வலது பக்கம் அதே tabs-ஐ தற்போதைய காட்சிக்கு சுற்றி நகர்த்தும்.",
    "layout.navigationTabs": "வழிசெலுத்தல் tabs",
    "layout.toolbarActions": "கருவிப்பட்டி செயல்கள்",
    "nav.calendar": "நாட்காட்டி",
    "nav.calendars": "நாட்காட்டிகள்",
    "nav.hideSidebar": "Sidebar-ஐ மறை",
    "nav.notes": "குறிப்புகள்",
    "nav.showAllCalendars": "அனைத்து நாட்காட்டிகளையும் காட்டு",
    "nav.showSidebar": "Sidebar-ஐ காட்டு",
    "nav.tasks": "பணிகள்",
    "settings.agentAccess": "Agent அணுகல்",
    "settings.appLanguage": "App மொழி",
    "settings.general": "பொது",
    "settings.language": "மொழி",
    "settings.layout": "அமைப்பு",
    "settings.startup": "தொடக்கம்",
    "settings.sync": "ஒத்திசைவு"
  },
  ms: {
    ...en,
    "action.apply": "Guna",
    "action.calendar": "Kalendar",
    "action.commandPalette": "Palet arahan",
    "action.diagnostics": "Diagnostik",
    "action.moveDown": "Alih ke bawah",
    "action.moveUp": "Alih ke atas",
    "action.notifications": "Pemberitahuan",
    "action.refresh": "Muat semula",
    "action.reset": "Tetap semula",
    "action.settings": "Tetapan",
    "action.splitView": "Paparan pisah",
    "action.viewDiagnostics": "Lihat diagnostik",
    "command.close": "Tutup palet arahan",
    "command.empty.noCommands": "Tiada arahan ditemui",
    "command.empty.noLocalResults": "Tiada hasil setempat",
    "command.empty.trySection": "Cuba nama seksyen atau tindakan.",
    "command.empty.unmatched": "Tiada arahan atau item cache sepadan dengan carian ini.",
    "command.filter": "Tapis arahan",
    "command.loading.detail": "Tugas, acara, nota, hari lahir dan teks kalendar.",
    "command.loading.title": "Mencari data perancang",
    "command.placeholder": "Jalankan arahan atau cari data perancang",
    "command.title": "Palet arahan",
    "diagnostics.description": "Semak log, sejarah mutasi, baris gilir segerak dan bundle sokongan.",
    "diagnostics.includeGooglePayloads": "Sertakan payload Google yang telah disunting medan dalam log setempat",
    "diagnostics.includeGooglePayloads.description": "Log setempat masa depan mungkin mengandungi petikan penyelesaian masalah Google yang telah disunting medan.",
    "diagnostics.includePerformance": "Sertakan diagnostik prestasi",
    "diagnostics.includePerformance.description": "Menyertakan masa permulaan, migrasi, pertanyaan perlahan dan permintaan MCP.",
    "diagnostics.title": "Diagnostik",
    "language.description": "System Default mengikut susunan bahasa macOS anda.",
    "language.english": "Bahasa Inggeris",
    "language.japanese": "Bahasa Jepun",
    "language.korean": "Bahasa Korea",
    "language.malay": "Bahasa Melayu",
    "language.simplifiedChinese": "Bahasa Cina Mandarin (Ringkas)",
    "language.system": "System Default",
    "language.tamil": "Bahasa Tamil",
    "layout.calendarViewModes": "Mod paparan kalendar",
    "layout.navigationPlacement": "Kedudukan navigasi",
    "layout.navigationPlacement.description": "Kiri mengekalkan kedudukan sidebar native; kanan mengalihkan tab yang sama di sekitar paparan semasa.",
    "layout.navigationTabs": "Tab navigasi",
    "layout.toolbarActions": "Tindakan bar alat",
    "nav.calendar": "Kalendar",
    "nav.calendars": "Kalendar",
    "nav.hideSidebar": "Sembunyikan sidebar",
    "nav.notes": "Nota",
    "nav.showAllCalendars": "Tunjukkan semua kalendar",
    "nav.showSidebar": "Tunjukkan sidebar",
    "nav.tasks": "Tugas",
    "settings.agentAccess": "Akses ejen",
    "settings.appLanguage": "Bahasa aplikasi",
    "settings.general": "Umum",
    "settings.language": "Bahasa",
    "settings.layout": "Susun atur",
    "settings.startup": "Permulaan",
    "settings.sync": "Segerak"
  },
  ko: {
    ...en,
    "action.apply": "적용",
    "action.calendar": "캘린더",
    "action.commandPalette": "명령 팔레트",
    "action.diagnostics": "진단",
    "action.moveDown": "아래로 이동",
    "action.moveUp": "위로 이동",
    "action.notifications": "알림",
    "action.refresh": "다시 불러오기",
    "action.reset": "재설정",
    "action.settings": "설정",
    "action.splitView": "분할 보기",
    "action.viewDiagnostics": "진단 보기",
    "command.close": "명령 팔레트 닫기",
    "command.empty.noCommands": "명령을 찾을 수 없음",
    "command.empty.noLocalResults": "로컬 결과 없음",
    "command.empty.trySection": "섹션 이름이나 작업을 입력해 보세요.",
    "command.empty.unmatched": "이 검색어와 일치하는 명령 또는 캐시 항목이 없습니다.",
    "command.filter": "명령 필터",
    "command.loading.detail": "작업, 이벤트, 노트, 생일 및 캘린더 텍스트.",
    "command.loading.title": "플래너 데이터 검색 중",
    "command.placeholder": "명령 실행 또는 플래너 데이터 검색",
    "command.title": "명령 팔레트",
    "diagnostics.description": "로그, 변경 기록, 동기화 대기열 및 지원 번들을 확인합니다.",
    "diagnostics.includeGooglePayloads": "로컬 로그에 필드가 수정된 Google 페이로드 포함",
    "diagnostics.includeGooglePayloads.description": "향후 로컬 로그에는 필드가 수정된 Google 문제 해결 조각이 포함될 수 있습니다.",
    "diagnostics.includePerformance": "성능 진단 포함",
    "diagnostics.includePerformance.description": "시작, 마이그레이션, 느린 쿼리, MCP 요청 시간을 포함합니다.",
    "diagnostics.title": "진단",
    "language.description": "System Default는 macOS 언어 순서를 따릅니다.",
    "language.english": "영어",
    "language.japanese": "일본어",
    "language.korean": "한국어",
    "language.malay": "말레이어",
    "language.simplifiedChinese": "중국어 만다린(간체)",
    "language.system": "System Default",
    "language.tamil": "타밀어",
    "layout.calendarViewModes": "캘린더 보기 모드",
    "layout.navigationPlacement": "탐색 위치",
    "layout.navigationPlacement.description": "왼쪽은 기본 사이드바 위치를 유지하고, 오른쪽은 같은 탭을 현재 보기 주변으로 이동합니다.",
    "layout.navigationTabs": "탐색 탭",
    "layout.toolbarActions": "도구 모음 작업",
    "nav.calendar": "캘린더",
    "nav.calendars": "캘린더",
    "nav.hideSidebar": "사이드바 숨기기",
    "nav.notes": "노트",
    "nav.showAllCalendars": "모든 캘린더 표시",
    "nav.showSidebar": "사이드바 표시",
    "nav.tasks": "작업",
    "settings.agentAccess": "에이전트 접근",
    "settings.appLanguage": "앱 언어",
    "settings.general": "일반",
    "settings.language": "언어",
    "settings.layout": "레이아웃",
    "settings.startup": "시작",
    "settings.sync": "동기화"
  },
  ja: {
    ...en,
    "action.apply": "適用",
    "action.calendar": "カレンダー",
    "action.commandPalette": "コマンドパレット",
    "action.diagnostics": "診断",
    "action.moveDown": "下へ移動",
    "action.moveUp": "上へ移動",
    "action.notifications": "通知",
    "action.refresh": "再読み込み",
    "action.reset": "リセット",
    "action.settings": "設定",
    "action.splitView": "分割表示",
    "action.viewDiagnostics": "診断を表示",
    "command.close": "コマンドパレットを閉じる",
    "command.empty.noCommands": "コマンドが見つかりません",
    "command.empty.noLocalResults": "ローカル結果がありません",
    "command.empty.trySection": "セクション名または操作を入力してください。",
    "command.empty.unmatched": "この検索に一致するコマンドまたはキャッシュ項目はありません。",
    "command.filter": "コマンドを絞り込み",
    "command.loading.detail": "タスク、予定、ノート、誕生日、カレンダーテキスト。",
    "command.loading.title": "プランナーデータを検索中",
    "command.placeholder": "コマンドを実行またはプランナーデータを検索",
    "command.title": "コマンドパレット",
    "diagnostics.description": "ログ、変更履歴、同期キュー、サポートバンドルを確認します。",
    "diagnostics.includeGooglePayloads": "フィールドを編集した Google ペイロードをローカルログに含める",
    "diagnostics.includeGooglePayloads.description": "今後のローカルログには、フィールドを編集した Google トラブルシューティング断片が含まれる場合があります。",
    "diagnostics.includePerformance": "パフォーマンス診断を含める",
    "diagnostics.includePerformance.description": "起動、移行、低速クエリ、MCP リクエストの時間を含めます。",
    "diagnostics.title": "診断",
    "language.description": "System Default は macOS の言語順序に従います。",
    "language.english": "英語",
    "language.japanese": "日本語",
    "language.korean": "韓国語",
    "language.malay": "マレー語",
    "language.simplifiedChinese": "中国語（簡体字）",
    "language.system": "System Default",
    "language.tamil": "タミル語",
    "layout.calendarViewModes": "カレンダー表示モード",
    "layout.navigationPlacement": "ナビゲーション位置",
    "layout.navigationPlacement.description": "左は標準のサイドバー位置を維持し、右は同じタブを現在の表示の反対側へ移動します。",
    "layout.navigationTabs": "ナビゲーションタブ",
    "layout.toolbarActions": "ツールバー操作",
    "nav.calendar": "カレンダー",
    "nav.calendars": "カレンダー",
    "nav.hideSidebar": "サイドバーを隠す",
    "nav.notes": "ノート",
    "nav.showAllCalendars": "すべてのカレンダーを表示",
    "nav.showSidebar": "サイドバーを表示",
    "nav.tasks": "タスク",
    "settings.agentAccess": "エージェントアクセス",
    "settings.appLanguage": "アプリの言語",
    "settings.general": "一般",
    "settings.language": "言語",
    "settings.layout": "レイアウト",
    "settings.startup": "起動",
    "settings.sync": "同期"
  }
};

const supportedLocales = Object.keys(dictionaries) as SupportedLocale[];

export function resolveAppLanguage(language: SettingsSnapshot["appLanguage"]): SupportedLocale {
  if (language !== "system") {
    return language;
  }

  const candidates = typeof navigator === "undefined"
    ? []
    : [navigator.language, ...(navigator.languages ?? [])];

  for (const candidate of candidates) {
    const locale = normalizeLocale(candidate);
    if (locale) {
      return locale;
    }
  }

  return "en";
}

export function languageOptions(t: (key: TranslationKey) => string): Array<{ label: string; value: SettingsSnapshot["appLanguage"] }> {
  return [
    { value: "system", label: t("language.system") },
    { value: "en", label: t("language.english") },
    { value: "zh-Hans", label: t("language.simplifiedChinese") },
    { value: "ta", label: t("language.tamil") },
    { value: "ms", label: t("language.malay") },
    { value: "ko", label: t("language.korean") },
    { value: "ja", label: t("language.japanese") }
  ];
}

const I18nContext = createContext<{ locale: SupportedLocale; t: (key: TranslationKey) => string }>({
  locale: "en",
  t: (key) => en[key]
});

export function I18nProvider({
  children,
  language
}: {
  children: ReactNode;
  language: SettingsSnapshot["appLanguage"];
}): JSX.Element {
  const locale = resolveAppLanguage(language);
  const value = useMemo(() => ({
    locale,
    t: (key: TranslationKey): string => dictionaries[locale][key] ?? en[key]
  }), [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): { locale: SupportedLocale; t: (key: TranslationKey) => string } {
  return useContext(I18nContext);
}

function normalizeLocale(value: string | undefined): SupportedLocale | null {
  const normalized = value?.toLowerCase();
  if (!normalized) {
    return null;
  }

  if (normalized.startsWith("zh")) {
    return "zh-Hans";
  }

  return supportedLocales.find((locale) => normalized === locale.toLowerCase() || normalized.startsWith(`${locale.toLowerCase()}-`)) ?? null;
}
