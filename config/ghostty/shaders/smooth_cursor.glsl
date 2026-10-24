// Quiet Ayu / macOS: короткий плавный след, нативный курсор всегда виден.
// Шейдер не смещает framebuffer и не запускает анимацию в покое.
const float DURATION = 0.20;
const float TRAIL_OPACITY = 0.36;

float boxDistance(vec2 point, vec2 center, vec2 halfSize) {
    vec2 d = abs(point - center) - halfSize;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
    fragColor = texture(iChannel0, fragCoord / iResolution.xy);
    float age = max(iTime - iTimeCursorChange, 0.0);
    if (iFocus == 0 || iCursorVisible == 0 || age >= DURATION) {
        return;
    }

    vec4 current = iCurrentCursor;
    vec4 previous = iPreviousCursor;
    vec2 travel = current.xy - previous.xy;
    // Только близкие движения в одной строке, без шлейфа через разные панели.
    if ((iCurrentCursorStyle != CURSORSTYLE_BAR
         && iCurrentCursorStyle != CURSORSTYLE_BLOCK)
        || current.z <= 0.0 || current.w <= 0.0
        || abs(travel.y) > 1.0 || abs(travel.x) < 1.0
        || abs(travel.x) > current.w * 4.0
        || iCurrentCursorStyle != iPreviousCursorStyle
        || iTimeCursorChange <= iTimeFocus) {
        return;
    }

    // В Metal +Y направлена вниз, current.y — нижний край курсора.
    // У block след идёт от задней кромки, а не от центра: иначе tmux/TUI
    // закрывает его своим курсором уже через первые 40 мс движения.
    float currentInset = min(current.z * 0.5, 1.5);
    float previousInset = min(previous.z * 0.5, 1.5);
    float currentEdge = travel.x > 0.0 ? currentInset : current.z - currentInset;
    float previousEdge = travel.x > 0.0 ? previousInset : previous.z - previousInset;
    vec2 start = previous.xy + vec2(previousEdge, -previous.w * 0.5);
    vec2 end = current.xy + vec2(currentEdge, -current.w * 0.5);
    float progress = clamp(age / DURATION, 0.0, 1.0);
    float eased = 1.0 - pow(1.0 - progress, 3.0);
    vec2 follower = mix(start, end, eased);
    // Тонкий голубой след догоняет курсор, не заменяя точную позицию ввода.
    float mask = 1.0 - smoothstep(0.0, 1.0,
        boxDistance(fragCoord, follower, vec2(1.5, current.w * 0.38)));
    if (fragCoord.x >= current.x && fragCoord.x <= current.x + current.z) {
        return;
    }
    // Маска тёмных поверхностей работает и с фоном tmux, и с фоном Ghostty.
    // Не сравниваем framebuffer с другим цветовым пространством uniform-фона.
    float brightness = max(fragColor.r, max(fragColor.g, fragColor.b));
    float backgroundOnly = 1.0 - smoothstep(0.12, 0.30, brightness);
    float fade = 1.0 - smoothstep(0.35, 1.0, progress);
    // Цвет эффекта берём из uniform, не из пикселя под block-курсором.
    // Палитра Ghostty и framebuffer используют native Display P3.
    vec3 accent = iCursorColor;
    fragColor.rgb = mix(fragColor.rgb, accent,
                        mask * backgroundOnly * fade * TRAIL_OPACITY);
}
