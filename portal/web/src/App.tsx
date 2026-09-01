import { useEffect, useState } from "react";
import { LinkButton } from "@cloudflare/kumo/components/button";
import { Surface } from "@cloudflare/kumo/components/surface";
import { Loader } from "@cloudflare/kumo/components/loader";
import { SignOut } from "@phosphor-icons/react";
import vscodeLogo from "./assets/vscode.svg";
import "./index.css";

const HEARTBEAT_MS = 30000;

type ContainerStatus = "running" | "stopped" | "missing" | "starting";

interface MeResponse {
  username: string;
  name: string;
  container: ContainerStatus;
}

const statusLabels: Record<ContainerStatus, string> = {
  running: "稼働中",
  stopped: "停止中",
  missing: "未作成",
  starting: "起動中",
};

function MonitorIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-14 w-14" fill="none" stroke="#0f172a" strokeWidth="1.5">
      <rect x="2" y="3" width="20" height="14" rx="2" />
      <path d="M8 21h8M12 17v4" strokeLinecap="round" />
    </svg>
  );
}

function App() {
  const [loggedIn, setLoggedIn] = useState<boolean | null>(null);
  const [user, setUser] = useState<MeResponse | null>(null);
  const [loading, setLoading] = useState<string | null>(null);

  const refreshStatus = async () => {
    try {
      const res = await fetch("/api/me", { credentials: "same-origin" });
      if (res.status === 401) {
        setLoggedIn(false);
        return;
      }
      const data: MeResponse = await res.json();
      setLoggedIn(true);
      setUser(data);
    } catch {
      setLoggedIn(false);
    }
  };

  useEffect(() => {
    refreshStatus();
    const id = setInterval(refreshStatus, 10000);
    const hb = setInterval(() => {
      fetch("/api/heartbeat", { credentials: "same-origin", method: "POST" }).catch(() => {});
    }, HEARTBEAT_MS);

    // ブラウザの戻るボタンでアプリ (noVNC / VS Code) から戻ったとき、
    // bfcache 復元で loading 状態が残るのを防ぐ
    const clearLoading = () => setLoading(null);
    window.addEventListener("pageshow", clearLoading);

    return () => {
      clearInterval(id);
      clearInterval(hb);
      window.removeEventListener("pageshow", clearLoading);
    };
  }, []);

  const launch = async (app: "vscode" | "desktop") => {
    setLoading("コンテナを準備中...");
    try {
      const res = await fetch(`/api/launch?app=${app}`, { credentials: "same-origin" });
      if (res.status === 401) {
        window.location.href = "/login";
        return;
      }
      const data = await res.json();
      if (res.status !== 200) {
        setLoading(`エラー: ${data.error || "不明なエラー"}`);
        return;
      }
      setLoading("起動しました。アプリを開いています...");
      window.location.href = data.proxy_url;
    } catch {
      setLoading("接続エラー");
    }
  };

  if (loggedIn === null) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-kumo-canvas">
        <Loader />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col bg-kumo-canvas text-kumo-default">
      <header className="flex items-center justify-between border-b border-kumo-border-subtle px-6 py-4">
        <span className="text-lg font-semibold">クラウド演習室</span>
        {loggedIn && (
          <LinkButton href="/logout" variant="secondary" size="sm">
            <SignOut size={16} weight="bold" />
            ログアウト
          </LinkButton>
        )}
      </header>

      <main className="flex flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
        {!loggedIn ? (
          <div className="flex flex-col items-center gap-3">
            <h1 className="text-2xl font-semibold">数学研究部 クラウド演習室</h1>
            <p className="text-kumo-subtle">iPad から使える開発環境</p>
            <LinkButton href="/login" size="lg" variant="primary">
              学校アカウントでログイン
            </LinkButton>
          </div>
        ) : (
          <>
            <h1 className="text-2xl font-semibold">{user?.name} さん、こんにちは</h1>
            <p className="text-kumo-subtle">
              コンテナ: {user ? statusLabels[user.container] : "..."}
            </p>
            <div className="flex flex-wrap justify-center gap-5">
              <Surface
                as="button"
                color="secondary"
                className="flex w-56 cursor-pointer flex-col items-center gap-3 p-8 text-center transition-transform hover:-translate-y-0.5"
                onClick={() => launch("vscode")}
              >
                <img src={vscodeLogo} alt="VS Code" className="h-14 w-14" />
                <span className="text-lg font-semibold">VS Code</span>
                <span className="text-sm text-kumo-subtle">エディタで開発</span>
              </Surface>
              <Surface
                as="button"
                color="secondary"
                className="flex w-56 cursor-pointer flex-col items-center gap-3 p-8 text-center transition-transform hover:-translate-y-0.5"
                onClick={() => launch("desktop")}
              >
                <MonitorIcon />
                <span className="text-lg font-semibold">デスクトップ</span>
                <span className="text-sm text-kumo-subtle">LXQt デスクトップ</span>
              </Surface>
            </div>
          </>
        )}
      </main>

      {loading && (
        <div className="fixed inset-0 z-10 flex flex-col items-center justify-center gap-4 bg-kumo-canvas/90 text-kumo-default">
          <Loader />
          <p>{loading}</p>
        </div>
      )}
    </div>
  );
}

export default App;
