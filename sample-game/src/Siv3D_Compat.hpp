#pragma once
#include <iostream>
#include <vector>
#include <string>
#include <cmath>
#include <random>
#include <sstream>
#include <algorithm>
#include <chrono>

#if defined(__EMSCRIPTEN__)
#include <emscripten.h>
#include <emscripten/html5.h>
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#else
// Linux Native fallback
#endif

// 基本型の定義 (Siv3D準拠)
using int32 = int;
using uint32 = unsigned int;
template <typename T>
using Array = std::vector<T>;
using String = std::u32string;

// ユーティリティ
inline double Clamp(double val, double min, double max) {
    return std::max(min, std::min(max, val));
}

inline double Random(double min, double max) {
    static std::mt19937 mt(std::random_device{}());
    std::uniform_real_distribution<double> dist(min, max);
    return dist(mt);
}

// Vec2
struct Vec2 {
    double x = 0.0;
    double y = 0.0;
    Vec2() = default;
    Vec2(double x, double y) : x(x), y(y) {}
    Vec2 operator+(const Vec2& o) const { return Vec2(x + o.x, y + o.y); }
    Vec2 operator-(const Vec2& o) const { return Vec2(x - o.x, y - o.y); }
    Vec2 operator*(double s) const { return Vec2(x * s, y * s); }
    double length() const { return std::sqrt(x * x + y * y); }
};

// ColorF
struct ColorF {
    double r = 1.0, g = 1.0, b = 1.0, a = 1.0;
    ColorF() = default;
    ColorF(double r, double g, double b, double a = 1.0) : r(r), g(g), b(b), a(a) {}
    uint32 toHex() const {
        uint32 ur = (uint32)(Clamp(r, 0, 1) * 255.0);
        uint32 ug = (uint32)(Clamp(g, 0, 1) * 255.0);
        uint32 ub = (uint32)(Clamp(b, 0, 1) * 255.0);
        return (ur << 16) | (ug << 8) | ub;
    }
};

inline ColorF HSV(double h, double s, double v) {
    double c = v * s;
    double x = c * (1.0 - std::abs(std::fmod(h / 60.0, 2) - 1.0));
    double m = v - c;
    double r = 0, g = 0, b = 0;
    if (h < 60) { r = c; g = x; b = 0; }
    else if (h < 120) { r = x; g = c; b = 0; }
    else if (h < 180) { r = 0; g = c; b = x; }
    else if (h < 240) { r = 0; g = x; b = c; }
    else if (h < 300) { r = x; g = 0; b = c; }
    else { r = c; g = 0; b = x; }
    return ColorF(r + m, g + m, b + m, 1.0);
}

namespace Palette {
    const ColorF White(1.0, 1.0, 1.0);
    const ColorF Black(0.0, 0.0, 0.0);
    const ColorF Skyblue(0.53, 0.81, 0.92);
    const ColorF Cyan(0.0, 1.0, 1.0);
    const ColorF Yellow(1.0, 1.0, 0.0);
    const ColorF Greenyellow(0.68, 1.0, 0.18);
    const ColorF Gray(0.5, 0.5, 0.5);
    const ColorF Lightgray(0.75, 0.75, 0.75);
    const ColorF Red(1.0, 0.0, 0.0);
}

// 内部グラフィックス/入力バックエンド状態
namespace Detail {
    inline int screenW = 800;
    inline int screenH = 600;
    inline ColorF bgColor(0.1, 0.1, 0.1);
    inline double deltaTime = 0.016;
    inline std::chrono::high_resolution_clock::time_point lastTime;

    inline Vec2 cursorPos(400, 300);
    inline bool mouseLDown = false;
    inline bool mouseLPressed = false;
    inline bool keyLeftPressed = false;
    inline bool keyRightPressed = false;
    inline bool keyAPressed = false;
    inline bool keyDPressed = false;
    inline bool keySpaceDown = false;
    inline bool keyEscapeDown = false;

#if defined(__EMSCRIPTEN__)
    EM_JS(void, js_init_canvas, (int w, int h), {
        var canvas = document.getElementById('canvas');
        if (!canvas) {
            canvas = document.createElement('canvas');
            canvas.id = 'canvas';
            document.body.appendChild(canvas);
        }
        canvas.width = w;
        canvas.height = h;
        window.ctx = canvas.getContext('2d');
        window.drawQueue = [];
    });

    EM_JS(void, js_clear_screen, (double r, double g, double b), {
        if (!window.ctx) return;
        var r8 = Math.floor(r * 255);
        var g8 = Math.floor(g * 255);
        var b8 = Math.floor(b * 255);
        window.ctx.fillStyle = 'rgb(' + r8 + ',' + g8 + ',' + b8 + ')';
        window.ctx.fillRect(0, 0, window.ctx.canvas.width, window.ctx.canvas.height);
    });

    EM_JS(void, js_draw_circle, (double x, double y, double r, double cr, double cg, double cb, double ca), {
        if (!window.ctx) return;
        window.ctx.beginPath();
        window.ctx.arc(x, y, r, 0, 2 * Math.PI, false);
        window.ctx.fillStyle = 'rgba(' + Math.floor(cr*255) + ',' + Math.floor(cg*255) + ',' + Math.floor(cb*255) + ',' + ca + ')';
        window.ctx.fill();
    });

    EM_JS(void, js_draw_rect, (double x, double y, double w, double h, double cr, double cg, double cb, double ca), {
        if (!window.ctx) return;
        window.ctx.fillStyle = 'rgba(' + Math.floor(cr*255) + ',' + Math.floor(cg*255) + ',' + Math.floor(cb*255) + ',' + ca + ')';
        window.ctx.fillRect(x, y, w, h);
    });

    EM_JS(void, js_draw_triangle, (double x1, double y1, double x2, double y2, double x3, double y3, double cr, double cg, double cb, double ca), {
        if (!window.ctx) return;
        window.ctx.beginPath();
        window.ctx.moveTo(x1, y1);
        window.ctx.lineTo(x2, y2);
        window.ctx.lineTo(x3, y3);
        window.ctx.closePath();
        window.ctx.fillStyle = 'rgba(' + Math.floor(cr*255) + ',' + Math.floor(cg*255) + ',' + Math.floor(cb*255) + ',' + ca + ')';
        window.ctx.fill();
    });

    EM_JS(void, js_draw_text, (const char* utf8Str, double x, double y, int fontSize, bool isBold, bool alignCenter, double cr, double cg, double cb), {
        if (!window.ctx) return;
        var prefix = isBold ? "bold " : "";
        window.ctx.font = prefix + fontSize + "px sans-serif";
        window.ctx.fillStyle = 'rgb(' + Math.floor(cr*255) + ',' + Math.floor(cg*255) + ',' + Math.floor(cb*255) + ')';
        if (alignCenter) {
            window.ctx.textAlign = 'center';
            window.ctx.textBaseline = 'middle';
        } else {
            window.ctx.textAlign = 'left';
            window.ctx.textBaseline = 'top';
        }
        window.ctx.fillText(text, x, y);
    });
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
    inline HWND hwnd = NULL;
    inline HDC hdcBack = NULL;
    inline HBITMAP hbmBack = NULL;
    inline HBITMAP hbmOld = NULL;

    LRESULT CALLBACK WndProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam) {
        switch (msg) {
            case WM_DESTROY:
                PostQuitMessage(0);
                return 0;
            case WM_MOUSEMOVE:
                cursorPos.x = (short)LOWORD(lParam);
                cursorPos.y = (short)HIWORD(lParam);
                return 0;
            case WM_LBUTTONDOWN:
                mouseLDown = true;
                mouseLPressed = true;
                cursorPos.x = (short)LOWORD(lParam);
                cursorPos.y = (short)HIWORD(lParam);
                return 0;
            case WM_LBUTTONUP:
                mouseLPressed = false;
                return 0;
            case WM_KEYDOWN:
                if (wParam == VK_LEFT) keyLeftPressed = true;
                if (wParam == VK_RIGHT) keyRightPressed = true;
                if (wParam == 'A') keyAPressed = true;
                if (wParam == 'D') keyDPressed = true;
                if (wParam == VK_SPACE) keySpaceDown = true;
                if (wParam == VK_ESCAPE) keyEscapeDown = true;
                return 0;
            case WM_KEYUP:
                if (wParam == VK_LEFT) keyLeftPressed = false;
                if (wParam == VK_RIGHT) keyRightPressed = false;
                if (wParam == 'A') keyAPressed = false;
                if (wParam == 'D') keyDPressed = false;
                return 0;
        }
        return DefWindowProc(hwnd, msg, wParam, lParam);
    }

    inline void init_win32_window(int w, int h) {
        WNDCLASS wc = {0};
        wc.lpfnWndProc = WndProc;
        wc.hInstance = GetModuleHandle(NULL);
        wc.lpszClassName = "Siv3D_Game_Window";
        wc.hCursor = LoadCursor(NULL, IDC_ARROW);
        RegisterClass(&wc);

        RECT rc = {0, 0, w, h};
        AdjustWindowRect(&rc, WS_OVERLAPPEDWINDOW & ~WS_THICKFRAME & ~WS_MAXIMIZEBOX, FALSE);
        hwnd = CreateWindow("Siv3D_Game_Window", "Siv3D Game (Windows Exhibition)",
            WS_OVERLAPPEDWINDOW & ~WS_THICKFRAME & ~WS_MAXIMIZEBOX | WS_VISIBLE,
            CW_USEDEFAULT, CW_USEDEFAULT, rc.right - rc.left, rc.bottom - rc.top,
            NULL, NULL, GetModuleHandle(NULL), NULL);

        HDC hdc = GetDC(hwnd);
        hdcBack = CreateCompatibleDC(hdc);
        hbmBack = CreateCompatibleBitmap(hdc, w, h);
        hbmOld = (HBITMAP)SelectObject(hdcBack, hbmBack);
        ReleaseDC(hwnd, hdc);
    }
#endif
}

// UTF32 to UTF8 helper
inline std::string ToUTF8(const std::u32string& u32) {
    std::string out;
    for (char32_t c : u32) {
        if (c < 0x80) {
            out.push_back((char)c);
        } else if (c < 0x800) {
            out.push_back((char)(0xC0 | ((c >> 6) & 0x1F)));
            out.push_back((char)(0x80 | (c & 0x3F)));
        } else if (c < 0x10000) {
            out.push_back((char)(0xE0 | ((c >> 12) & 0x0F)));
            out.push_back((char)(0x80 | ((c >> 6) & 0x3F)));
            out.push_back((char)(0x80 | (c & 0x3F)));
        } else {
            out.push_back((char)(0xF0 | ((c >> 18) & 0x07)));
            out.push_back((char)(0x80 | ((c >> 12) & 0x3F)));
            out.push_back((char)(0x80 | ((c >> 6) & 0x3F)));
            out.push_back((char)(0x80 | (c & 0x3F)));
        }
    }
    return out;
}

// UTF32 to UTF16 (std::wstring for Windows Unicode GDI)
inline std::wstring ToWide(const std::u32string& u32) {
    std::wstring out;
    for (char32_t c : u32) {
        if (c <= 0xFFFF) {
            out.push_back((wchar_t)c);
        } else if (c <= 0x10FFFF) {
            c -= 0x10000;
            out.push_back((wchar_t)((c >> 10) + 0xD800));
            out.push_back((wchar_t)((c & 0x3FF) + 0xDC00));
        }
    }
    return out;
}

inline std::u32string Format(const char32_t* prefix, int32 value) {
    std::u32string s(prefix);
    std::string v = std::to_string(value);
    for (char c : v) s.push_back((char32_t)c);
    return s;
}

// Scene
struct Scene {
    static void Resize(int w, int h) {
        Detail::screenW = w;
        Detail::screenH = h;
    }
    static void SetBackground(const ColorF& color) {
        Detail::bgColor = color;
    }
    static double DeltaTime() {
        return Detail::deltaTime;
    }
};

// Shape & Drawing
struct Circle {
    Vec2 center;
    double r;
    Circle(const Vec2& center, double r) : center(center), r(r) {}
    Circle(double x, double y, double r) : center(x, y), r(r) {}
    
    bool intersects(const Circle& o) const {
        return (center - o.center).length() <= (r + o.r);
    }

    void draw(const ColorF& color = Palette::White) const {
#if defined(__EMSCRIPTEN__)
        Detail::js_draw_circle(center.x, center.y, r, color.r, color.g, color.b, color.a);
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
        if (Detail::hdcBack) {
            COLORREF c = RGB((int)(color.r * 255), (int)(color.g * 255), (int)(color.b * 255));
            HBRUSH brush = CreateSolidBrush(c);
            HPEN pen = CreatePen(PS_SOLID, 1, c);
            HBRUSH oldB = (HBRUSH)SelectObject(Detail::hdcBack, brush);
            HPEN oldP = (HPEN)SelectObject(Detail::hdcBack, pen);
            Ellipse(Detail::hdcBack, (int)(center.x - r), (int)(center.y - r), (int)(center.x + r), (int)(center.y + r));
            SelectObject(Detail::hdcBack, oldB);
            SelectObject(Detail::hdcBack, oldP);
            DeleteObject(brush);
            DeleteObject(pen);
        }
#endif
    }
};

struct Triangle {
    Vec2 p1, p2, p3;
    Triangle(const Vec2& p1, const Vec2& p2, const Vec2& p3) : p1(p1), p2(p2), p3(p3) {}

    void draw(const ColorF& color = Palette::White) const {
#if defined(__EMSCRIPTEN__)
        Detail::js_draw_triangle(p1.x, p1.y, p2.x, p2.y, p3.x, p3.y, color.r, color.g, color.b, color.a);
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
        if (Detail::hdcBack) {
            COLORREF c = RGB((int)(color.r * 255), (int)(color.g * 255), (int)(color.b * 255));
            HBRUSH brush = CreateSolidBrush(c);
            HPEN pen = CreatePen(PS_SOLID, 1, c);
            HBRUSH oldB = (HBRUSH)SelectObject(Detail::hdcBack, brush);
            HPEN oldP = (HPEN)SelectObject(Detail::hdcBack, pen);
            POINT pts[3] = { {(LONG)p1.x, (LONG)p1.y}, {(LONG)p2.x, (LONG)p2.y}, {(LONG)p3.x, (LONG)p3.y} };
            Polygon(Detail::hdcBack, pts, 3);
            SelectObject(Detail::hdcBack, oldB);
            SelectObject(Detail::hdcBack, oldP);
            DeleteObject(brush);
            DeleteObject(pen);
        }
#endif
    }
};

enum class Typeface { Regular, Bold };

struct Font {
    int fontSize;
    Typeface typeface;
    Font(int size = 20, Typeface tf = Typeface::Regular) : fontSize(size), typeface(tf) {}

    void draw(const std::u32string& text, const Vec2& pos, const ColorF& color = Palette::White) const {
#if defined(__EMSCRIPTEN__)
        std::string s = ToUTF8(text);
        Detail::js_draw_text(s.c_str(), pos.x, pos.y, fontSize, typeface == Typeface::Bold, false, color.r, color.g, color.b);
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
        if (Detail::hdcBack) {
            std::wstring ws = ToWide(text);
            SetBkMode(Detail::hdcBack, TRANSPARENT);
            SetTextColor(Detail::hdcBack, RGB((int)(color.r * 255), (int)(color.g * 255), (int)(color.b * 255)));
            HFONT hf = CreateFontW(fontSize, 0, 0, 0, typeface == Typeface::Bold ? FW_BOLD : FW_NORMAL,
                                  FALSE, FALSE, FALSE, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS,
                                  CLIP_DEFAULT_PRECIS, DEFAULT_QUALITY, DEFAULT_PITCH | FF_DONTCARE, L"Yu Gothic UI");
            HFONT oldF = (HFONT)SelectObject(Detail::hdcBack, hf);
            TextOutW(Detail::hdcBack, (int)pos.x, (int)pos.y, ws.c_str(), (int)ws.length());
            SelectObject(Detail::hdcBack, oldF);
            DeleteObject(hf);
        }
#endif
    }

    void drawAt(const std::u32string& text, const Vec2& pos, const ColorF& color = Palette::White) const {
#if defined(__EMSCRIPTEN__)
        std::string s = ToUTF8(text);
        Detail::js_draw_text(s.c_str(), pos.x, pos.y, fontSize, typeface == Typeface::Bold, true, color.r, color.g, color.b);
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
        if (Detail::hdcBack) {
            std::wstring ws = ToWide(text);
            SetBkMode(Detail::hdcBack, TRANSPARENT);
            SetTextColor(Detail::hdcBack, RGB((int)(color.r * 255), (int)(color.g * 255), (int)(color.b * 255)));
            HFONT hf = CreateFontW(fontSize, 0, 0, 0, typeface == Typeface::Bold ? FW_BOLD : FW_NORMAL,
                                  FALSE, FALSE, FALSE, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS,
                                  CLIP_DEFAULT_PRECIS, DEFAULT_QUALITY, DEFAULT_PITCH | FF_DONTCARE, L"Yu Gothic UI");
            HFONT oldF = (HFONT)SelectObject(Detail::hdcBack, hf);
            SIZE sz;
            GetTextExtentPoint32W(Detail::hdcBack, ws.c_str(), (int)ws.length(), &sz);
            TextOutW(Detail::hdcBack, (int)(pos.x - sz.cx / 2), (int)(pos.y - sz.cy / 2), ws.c_str(), (int)ws.length());
            SelectObject(Detail::hdcBack, oldF);
            DeleteObject(hf);
        }
#endif
    }
};

// Input
struct Cursor {
    static Vec2 Pos() { return Detail::cursorPos; }
};

struct KeyState {
    bool* pPressed;
    bool* pDown;
    bool pressed() const { return pPressed && *pPressed; }
    bool down() const {
        if (pDown && *pDown) {
            *pDown = false; // consume
            return true;
        }
        return false;
    }
};

inline KeyState MouseL{&Detail::mouseLPressed, &Detail::mouseLDown};
inline KeyState KeyLeft{&Detail::keyLeftPressed, nullptr};
inline KeyState KeyRight{&Detail::keyRightPressed, nullptr};
inline KeyState KeyA{&Detail::keyAPressed, nullptr};
inline KeyState KeyD{&Detail::keyDPressed, nullptr};
inline KeyState KeySpace{nullptr, &Detail::keySpaceDown};
inline KeyState KeyEscape{nullptr, &Detail::keyEscapeDown};

// System
void Main();

struct System {
    static bool Update() {
#if defined(__EMSCRIPTEN__)
        // 描画バッファのフラッシュ & 次フレームへ
        return true;
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
        // Windows ダブルバッファ転送 & メッセージループ
        if (Detail::hwnd && Detail::hdcBack) {
            HDC hdc = GetDC(Detail::hwnd);
            BitBlt(hdc, 0, 0, Detail::screenW, Detail::screenH, Detail::hdcBack, 0, 0, SRCCOPY);
            ReleaseDC(Detail::hwnd, hdc);

            // クリア
            HBRUSH bg = CreateSolidBrush(RGB((int)(Detail::bgColor.r * 255), (int)(Detail::bgColor.g * 255), (int)(Detail::bgColor.b * 255)));
            RECT rc = {0, 0, Detail::screenW, Detail::screenH};
            FillRect(Detail::hdcBack, &rc, bg);
            DeleteObject(bg);
        }

        MSG msg;
        while (PeekMessage(&msg, NULL, 0, 0, PM_REMOVE)) {
            if (msg.message == WM_QUIT) return false;
            TranslateMessage(&msg);
            DispatchMessage(&msg);
        }
        Sleep(16);
        return true;
#else
        return false;
#endif
    }
};

#if defined(__EMSCRIPTEN__)
// Emscripten メインループ
extern "C" {
    EMSCRIPTEN_KEEPALIVE
    void em_on_mouse_move(double x, double y) {
        Detail::cursorPos.x = x;
        Detail::cursorPos.y = y;
    }
    EMSCRIPTEN_KEEPALIVE
    void em_on_mouse_down(double x, double y) {
        Detail::cursorPos.x = x;
        Detail::cursorPos.y = y;
        Detail::mouseLDown = true;
        Detail::mouseLPressed = true;
    }
    EMSCRIPTEN_KEEPALIVE
    void em_on_mouse_up() {
        Detail::mouseLPressed = false;
    }
    EMSCRIPTEN_KEEPALIVE
    void em_on_key_down(int code) {
        if (code == 37) Detail::keyLeftPressed = true;
        if (code == 39) Detail::keyRightPressed = true;
        if (code == 65) Detail::keyAPressed = true;
        if (code == 68) Detail::keyDPressed = true;
        if (code == 32) Detail::keySpaceDown = true;
        if (code == 27) Detail::keyEscapeDown = true;
    }
    EMSCRIPTEN_KEEPALIVE
    void em_on_key_up(int code) {
        if (code == 37) Detail::keyLeftPressed = false;
        if (code == 39) Detail::keyRightPressed = false;
        if (code == 65) Detail::keyAPressed = false;
        if (code == 68) Detail::keyDPressed = false;
    }
}

void emscripten_loop() {
    Detail::js_clear_screen(Detail::bgColor.r, Detail::bgColor.g, Detail::bgColor.b);
    Main();
}

int main() {
    Detail::js_init_canvas(Detail::screenW, Detail::screenH);
    Detail::lastTime = std::chrono::high_resolution_clock::now();
    emscripten_set_main_loop(emscripten_loop, 0, 1);
    return 0;
}
#elif defined(_WIN32) || defined(__MINGW32__) || defined(__MINGW64__)
int WINAPI WinMain(HINSTANCE, HINSTANCE, LPSTR, int) {
    Detail::init_win32_window(Detail::screenW, Detail::screenH);
    Detail::lastTime = std::chrono::high_resolution_clock::now();
    
    // 背景クリア初期化
    HBRUSH bg = CreateSolidBrush(RGB((int)(Detail::bgColor.r * 255), (int)(Detail::bgColor.g * 255), (int)(Detail::bgColor.b * 255)));
    RECT rc = {0, 0, Detail::screenW, Detail::screenH};
    FillRect(Detail::hdcBack, &rc, bg);
    DeleteObject(bg);

    Main();
    return 0;
}
#endif
