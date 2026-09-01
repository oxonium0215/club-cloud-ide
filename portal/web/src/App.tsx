import { useEffect, useState } from "react";
import { Button, LinkButton } from "@cloudflare/kumo/components/button";
import { Surface } from "@cloudflare/kumo/components/surface";
import { Loader } from "@cloudflare/kumo/components/loader";
import { SignOut } from "@phosphor-icons/react";
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

function VSCodeIcon() {
  return (
    <svg viewBox="0 0 32 32" className="h-14 w-14">
      <path
        fill="#007acc"
        d="M23.2 1.4c-.6-.2-1.2-.1-1.7.2L3.1 13.5c-.6.4-.7 1.2-.3 1.8l1.7 2.2c.3.4.8.6 1.3.5l19.2-4.2c.6-.1 1.1-.6 1.1-1.2V3.2c0-.8-.6-1.5-1.3-1.7l-1.6-.1zm0 29.2c-.6.2-1.2.1-1.7-.2L3.1 18.5c-.6-.4-.7-1.2-.3-1.8l1.7-2.2c.3-.4.8-.6 1.3-.5l19.2 4.2c.6.1 1.1.6 1.1 1.2v8.6c0 .8-.6 1.5-1.3 1.7l-1.6.1z"
      />
      <path
        fill="#fff"
        d="M23.2 1.4c-.6-.2-1.2-.1-1.7.2L3.1 13.5c-.6.4-.7 1.2-.3 1.8l1.7 2.2c.3.4.8.6 1.3.5l19.2-4.2c.6-.1 1.1-.6 1.1-1.2V3.2c0-.8-.6-1.5-1.3-1.7l-1.6-.1zm0 29.2c-.6.2-1.2.1-1.7-.2L3.1 18.5c-.6-.4-.7-1.2-.3-1.8l1.7-2.2c.3-.4.8-.6 1.3-.5l19.2 4.2c.6.1 1.1.6 1.1 1.2v8.6c0 .8-.6 1.5-1.3 1.7l-1.6.1z"
        transform="scale(0.5) translate(16 16)"
      />
    </svg>
  );
}

function LXQtIcon() {
  return (
    <svg viewBox="0 0 32 32" className="h-14 w-14">
      <path
        fill="#17a2b8"
        d="M27.7 7.1L17 4.4c-.3-.1-.7-.1-1 0L5.3 7.1c-.7.2-1.2.8-1.2 1.5v14.8c0 .7.5 1.3 1.2 1.5l10.7 2.7c.3.1.7.1 1 0l10.7-2.7c.7-.2 1.2-.8 1.2-1.5V8.6c0-.7-.5-1.3-1.2-1.5zM9.3 12.1l-3-1v-2l3 1v2zm5 1.2l-4-1.3V9.9l4 1.3v2.1zm5.2 1.5l-4.2-1.4V10.9l4.2 1.4v2.5zm-15.3-5.9l10-3.3 10 3.3-10 2.5-10-2.5zm0 8.6l10 2.5v2.1l-10-2.5v-2.1zm11 2.5l10-2.5v2.1l-10 2.5v-2.1z"
        transform="scale(1.15) translate(-1 -1)"
      />
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
    return () => {
      clearInterval(id);
      clearInterval(hb);
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
              <Surface className="flex flex-col items-center gap-3 p-8">
                <VSCodeIcon />
                <Button size="lg" variant="secondary" onClick={() => launch("vscode")}>
                  VS Code
                </Button>
                <span className="text-sm text-kumo-subtle">エディタで開発</span>
              </Surface>
              <Surface className="flex flex-col items-center gap-3 p-8">
                <LXQtIcon />
                <Button size="lg" variant="secondary" onClick={() => launch("desktop")}>
                  デスクトップ
                </Button>
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
