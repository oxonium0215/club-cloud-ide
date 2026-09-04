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
  const [discordEnabled, setDiscordEnabled] = useState(false);

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
    // 公開設定 (Discord ログインの有無) を取得
    fetch("/api/config", { credentials: "same-origin" })
      .then((r) => r.json())
      .then((d) => setDiscordEnabled(!!d.discord_enabled))
      .catch(() => {});
  }, []);

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

      // コンテナ内サービス (code-server / noVNC) の起動を待つ。
      // 起動直後は 502 になるため、readiness を確認してから遷移する。
      const proxyUrl = data.proxy_url as string;
      for (let i = 0; i < 30; i++) {
        try {
          const check = await fetch(proxyUrl, {
            credentials: "same-origin",
            method: "GET",
            // noVNC は WebSocket で接続するため、HTML を確認するだけでは不十分。
            // ここでは Caddy が 502 以外 (200/302) を返せば準備完了とみなす。
          });
          if (check.status !== 502) {
            window.location.href = proxyUrl;
            return;
          }
        } catch {
          // ネットワークエラーも起動中とみなして再試行
        }
        await new Promise((r) => setTimeout(r, 2000));
      }
      // タイムアウト: それでも繋がらない場合は遷移 (再試行はブラウザ任せ)
      window.location.href = proxyUrl;
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
            {discordEnabled && (
              <LinkButton href="/login/discord" size="lg" variant="secondary">
                <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor" aria-hidden>
                  <path d="M20.317 4.37a19.79 19.79 0 0 0-4.885-1.515a.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0a12.64 12.64 0 0 0-.617-1.25a.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057a19.9 19.9 0 0 0 5.993 3.03a.078.078 0 0 0 .084-.028a14.09 14.09 0 0 0 1.226-1.994a.076.076 0 0 0-.041-.106a13.107 13.107 0 0 1-1.872-.892a.077.077 0 0 1-.008-.128a10.2 10.2 0 0 0 .372-.292a.074.074 0 0 1 .077-.01c3.928 1.793 8.18 1.793 12.062 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127a12.3 12.3 0 0 1-1.873.892a.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028a19.84 19.84 0 0 0 6.002-3.03a.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.03zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419c0-1.333.956-2.419 2.157-2.419c1.21 0 2.176 1.096 2.157 2.42c0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419c0-1.333.955-2.419 2.157-2.419c1.21 0 2.176 1.096 2.157 2.42c0 1.333-.946 2.418-2.157 2.418z" />
                </svg>
                Discordでログイン
              </LinkButton>
            )}
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
