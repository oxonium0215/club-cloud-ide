import { useEffect, useState } from "react";
import { Button, LinkButton } from "@cloudflare/kumo/components/button";
import { Code, Desktop, SignOut } from "@phosphor-icons/react";
import "./index.css";
import "./App.css";

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
      <div className="center">
        <div className="spinner" />
      </div>
    );
  }

  return (
    <div className="page">
      <header className="header">
        <div className="brand">
          <span className="brand-name">クラウド演習室</span>
        </div>
        {loggedIn && (
          <a href="/logout" className="logout">
            <SignOut size={16} weight="bold" />
            ログアウト
          </a>
        )}
      </header>

      <main className="main">
        {!loggedIn ? (
          <div className="login-box">
            <h1>数学研究部 クラウド演習室</h1>
            <p className="sub">iPad から使える開発環境</p>
            <LinkButton href="/login" size="lg">
              学校アカウントでログイン
            </LinkButton>
          </div>
        ) : (
          <>
            <h1>{user?.name} さん、こんにちは</h1>
            <p className="sub">
              コンテナ: {user ? statusLabels[user.container] : "..."}
            </p>
            <div className="apps">
              <Button
                size="lg"
                className="app-card"
                onClick={() => launch("vscode")}
              >
                <Code size={32} weight="bold" />
                <div className="app-label">VS Code</div>
                <div className="app-desc">エディタで開発</div>
              </Button>
              <Button
                size="lg"
                className="app-card"
                onClick={() => launch("desktop")}
              >
                <Desktop size={32} weight="bold" />
                <div className="app-label">デスクトップ</div>
                <div className="app-desc">LXQt デスクトップ</div>
              </Button>
            </div>
          </>
        )}
      </main>

      {loading && (
        <div className="loading">
          <div className="spinner" />
          <p>{loading}</p>
        </div>
      )}
    </div>
  );
}

export default App;
