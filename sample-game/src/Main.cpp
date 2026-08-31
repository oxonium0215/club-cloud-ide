// Main.cpp: 数学研究部 Siv3D サンプルゲーム（単一ソースでWeb/Windows両対応）
#include "Siv3D_Compat.hpp"

// ゲームの状態管理
enum class GameState { Title, Playing, GameOver };

void Main() {
    // 画面サイズ設定 (iPad/ブラウザ/Windows共通)
    Scene::Resize(800, 600);
    Scene::SetBackground(ColorF(0.1, 0.12, 0.18));

    GameState state = GameState::Title;
    int32 score = 0;
    double playerX = 400.0;
    double playerY = 500.0;
    const double playerSpeed = 350.0;

    struct Target {
        Vec2 pos;
        double speed;
        ColorF color;
        bool active;
    };

    Array<Target> targets;
    double spawnTimer = 0.0;

    Font fontTitle(32, Typeface::Bold);
    Font fontScore(24);
    Font fontMsg(20);

    while (System::Update()) {
        const double dt = Scene::DeltaTime();

        // 状態ごとの処理
        if (state == GameState::Title) {
            fontTitle.drawAt(U"Siv3D Shooting Game", Vec2(400, 200), Palette::Skyblue);
            fontMsg.drawAt(U"数学研究部 クラウド開発環境 デモ", Vec2(400, 260), Palette::White);
            
            #if defined(__EMSCRIPTEN__)
            fontMsg.drawAt(U"[ iPad / WebAssembly 実行中 ]", Vec2(400, 320), Palette::Greenyellow);
            fontMsg.drawAt(U"画面タップ / クリックでスタート", Vec2(400, 420), Palette::Yellow);
            #else
            fontMsg.drawAt(U"[ Windows .exe 実行中 ]", Vec2(400, 320), Palette::Greenyellow);
            fontMsg.drawAt(U"スペースキー / クリックでスタート", Vec2(400, 420), Palette::Yellow);
            #endif

            if (KeySpace.down() || MouseL.down()) {
                state = GameState::Playing;
                score = 0;
                playerX = 400.0;
                targets.clear();
            }
        }
        else if (state == GameState::Playing) {
            // 自機操作（キーボードまたはタッチ/マウス）
            if (KeyLeft.pressed() || KeyA.pressed()) {
                playerX -= playerSpeed * dt;
            }
            if (KeyRight.pressed() || KeyD.pressed()) {
                playerX += playerSpeed * dt;
            }
            if (MouseL.pressed()) {
                // マウス/タッチ位置へのスムーズ追従
                playerX += (Cursor::Pos().x - playerX) * 10.0 * dt;
            }

            playerX = Clamp(playerX, 30.0, 770.0);

            // ターゲット生成
            spawnTimer += dt;
            if (spawnTimer >= 0.8) {
                spawnTimer = 0.0;
                Target t;
                t.pos = Vec2(Random(50.0, 750.0), -20.0);
                t.speed = Random(120.0, 260.0);
                t.color = HSV(Random(0.0, 360.0), 0.8, 0.95);
                t.active = true;
                targets.push_back(t);
            }

            // ターゲット更新 & 描画
            for (auto& t : targets) {
                if (!t.active) continue;
                t.pos.y += t.speed * dt;

                // 描画
                Circle(t.pos, 22.0).draw(t.color);

                // 自機との当たり判定
                if (Circle(Vec2(playerX, playerY), 25.0).intersects(Circle(t.pos, 22.0))) {
                    t.active = false;
                    score += 10;
                }

                // 画面外
                if (t.pos.y > 620.0) {
                    t.active = false;
                }
            }

            // 自機描画 (三角ロケット)
            Triangle(Vec2(playerX, playerY - 30), Vec2(playerX - 25, playerY + 20), Vec2(playerX + 25, playerY + 20))
                .draw(Palette::Cyan);
            Circle(Vec2(playerX, playerY), 10).draw(Palette::White);

            // スコア描画
            fontScore.draw(Format(U"Score: ", score), Vec2(20, 20), Palette::White);
            fontMsg.draw(U"[ESC] タイトルへ", Vec2(650, 20), Palette::Gray);

            if (KeyEscape.down()) {
                state = GameState::Title;
            }
        }

        // フッター情報
        #if defined(__EMSCRIPTEN__)
        fontMsg.draw(U"iPad Safari/Chrome | WebAssembly build", Vec2(20, 565), Palette::Lightgray);
        #else
        fontMsg.draw(U"Windows展示用 x86_64 PE32+ build", Vec2(20, 565), Palette::Lightgray);
        #endif
    }
}
