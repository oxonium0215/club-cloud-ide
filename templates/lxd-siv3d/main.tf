terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 0.13.0"
    }
    lxd = {
      source  = "terraform-lxd/lxd"
      version = "~> 2.4.0"
    }
  }
}

variable "step" {
  description = "Workspace lifecycle step"
  default     = 1
}

variable "repo_url" {
  description = "設定配布元の Git リポジトリ URL (GitOps)"
  type        = string
  default     = "https://github.com/oxonium0215/club-cloud-ide.git"
}

variable "repo_ref" {
  description = "設定配布元のブランチ/タグ"
  type        = string
  default     = "main"
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  arch           = "amd64"
  os             = "linux"
  startup_script = <<-EOT
    #!/bin/bash
    set -e

    REPO_URL="${var.repo_url}"
    REPO_REF="${var.repo_ref}"

    # 1. 設定リポジトリの取得 (GitOps: 起動時に最新設定を配布)
    #    apply.sh が設定を /usr/local/bin 等へ配布し、code-server の導入も行う。
    #    初回のみ clone、以降は fetch して更新を反映する。
    if [ ! -d /home/coder/.config/club-cloud-ide/.git ]; then
      git clone --depth 1 --branch "$REPO_REF" "$REPO_URL" /home/coder/.config/club-cloud-ide
    else
      git -C /home/coder/.config/club-cloud-ide fetch --depth 1 origin "$REPO_REF"
      git -C /home/coder/.config/club-cloud-ide reset --hard FETCH_HEAD
    fi

    # 2. 設定配布 (sudo で /usr/local/bin や code-server の導入まで行う)
    sudo /home/coder/.config/club-cloud-ide/templates/lxd-siv3d/files/apply.sh

    # 3. デスクトップ & エディタ起動
    if [ -f /usr/local/bin/entrypoint.sh ]; then
      /usr/local/bin/entrypoint.sh &
    else
      code-server --auth none --port 13337 /home/coder/workspace &
    fi
  EOT

  metadata {
    display_name = "CPU Usage"
    key          = "cpu"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Memory Usage"
    key          = "mem"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }
}

resource "coder_app" "code_server" {
  agent_id     = coder_agent.main.id
  slug         = "vscode"
  display_name = "VS Code (エディタ)"
  icon         = "/icon/code.svg"
  url          = "http://localhost:13337/?folder=/home/coder/workspace"
  subdomain    = false
  share        = "owner"
}

resource "coder_app" "novnc_desktop" {
  agent_id     = coder_agent.main.id
  slug         = "desktop"
  display_name = "Linux デスクトップ (KDE Plasma)"
  icon         = "/icon/desktop.svg"
  url          = "http://localhost:6080/vnc.html?autoconnect=true&resize=remote"
  subdomain    = false
  share        = "owner"
}

# LXD システムコンテナ (一人一台の専用Linuxマシン、完全永続化)
# コンテナ名を owner 名に固定することで、1人1ワークスペースを強制する
# (2つ目のワークスペースを作ろうとすると LXD コンテナ名が衝突して失敗する)
resource "lxd_instance" "workspace" {
  name      = "osgsuken-${data.coder_workspace.me.owner}"
  image     = "osgsuken-base-img"
  ephemeral = false
  running   = data.coder_workspace.me.start_count > 0

  config = {
    "boot.autostart"   = "false"
    "security.nesting" = "true"
    "user.user-data"   = <<-EOT
      #cloud-config
      users:
        - name: coder
          sudo: ALL=(ALL) NOPASSWD:ALL
          shell: /bin/bash
      # ベースイメージに開発環境一式が入っているため、追加パッケージは不要
      packages: []
      runcmd:
        - sudo -u coder -H sh -c '${coder_agent.main.init_script}'
    EOT
  }

  limits = {
    cpu       = "2"
    memory    = "4GB"
    # KDE Plasma セッションは 200 プロセス程度消費するため、
    # デフォルトの processes 制限 (約 250) ではアプリ起動時に枯渇して
    # セッションごと落ちることがある。1024 に引き上げる。
    processes = "1024"
  }
}
