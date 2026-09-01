package main

// cloudInitData はコンテナ作成時に cloud-init で注入する設定。
// GitOps: 起動時にリポジトリから最新設定を pull し、サービスを起動する。
const cloudInitData = `#cloud-config
users:
  - name: osgsuken
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
packages: []
runcmd:
  - [sh, -c, "if [ ! -d /home/osgsuken/.config/club-cloud-ide/.git ]; then git clone --depth 1 --branch main REPO_URL_PLACEHOLDER /home/osgsuken/.config/club-cloud-ide; else git -C /home/osgsuken/.config/club-cloud-ide pull --ff-only; fi"]
  - [sh, -c, "chown -R osgsuken:osgsuken /home/osgsuken/.config/club-cloud-ide"]
  # apply.sh が設定配布と systemd サービス (workspace) の起動まで行う
  - [bash, /home/osgsuken/.config/club-cloud-ide/templates/lxd-siv3d/files/apply.sh]
`
