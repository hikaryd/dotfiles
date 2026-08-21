// Input motion for Ghostty 1.3+.
//
// Ghostty exposes the current framebuffer plus the current/previous cursor
// rectangles. That is enough to animate the newest glyph and to emit a clear
// deletion burst, but not to retain the pixels of a glyph after it is deleted.

const vec3 GOLD = vec3(0.902, 0.706, 0.314);
const vec3 ICE = vec3(0.435, 0.745, 0.925);

const float TYPE_DURATION = 0.38;
const float DELETE_DURATION = 0.68;
const float ENTER_DURATION = 0.52;
const float TYPE_RISE_PX = 9.0;

float easeOutQuint(float t) {
    float q = 1.0 - clamp(t, 0.0, 1.0);
    return 1.0 - q * q * q * q * q;
}

float easeInOutCubic(float t) {
    t = clamp(t, 0.0, 1.0);
    return t < 0.5
        ? 4.0 * t * t * t
        : 1.0 - pow(-2.0 * t + 2.0, 3.0) * 0.5;
}

float sdBox(vec2 point, vec2 center, vec2 halfSize) {
    vec2 d = abs(point - center) - halfSize;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}

float circleMask(vec2 point, vec2 center, float radius) {
    return 1.0 - smoothstep(radius, radius + 1.25, distance(point, center));
}

float segmentMask(vec2 point, vec2 start, vec2 end, float width) {
    vec2 line = end - start;
    float denom = max(dot(line, line), 0.0001);
    float along = clamp(dot(point - start, line) / denom, 0.0, 1.0);
    float distanceToLine = length(point - (start + line * along));
    return 1.0 - smoothstep(width, width + 1.0, distanceToLine);
}

float foregroundMask(vec3 color) {
    return smoothstep(0.025, 0.11, length(color - iBackgroundColor));
}

vec2 cursorCenter(vec4 cursor) {
    return cursor.xy + vec2(cursor.z * 0.5, -cursor.w * 0.5);
}

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
    vec2 uv = fragCoord / iResolution.xy;
    fragColor = texture(iChannel0, uv);

    if (iFocus == 0) {
        return;
    }

    vec4 current = iCurrentCursor;
    vec4 previous = iPreviousCursor;
    vec2 currentCenter = cursorCenter(current);
    vec2 previousCenter = cursorCenter(previous);
    vec2 delta = currentCenter - previousCenter;
    float age = max(iTime - iTimeCursorChange, 0.0);

    float cellW = max(previous.z, 1.0);
    float cellH = max(previous.w, 1.0);
    float sameRow = 1.0 - smoothstep(cellH * 0.12, cellH * 0.38, abs(delta.y));
    // Cursor rectangles vary slightly between apps and cursor styles. Accept
    // any clear one-cell-ish horizontal move rather than demanding exact width.
    float horizontalMove = step(cellW * 0.24, abs(delta.x))
        * (1.0 - step(cellW * 1.85, abs(delta.x)));
    float oneRight = horizontalMove * step(0.0, delta.x);
    float oneLeft = horizontalMove * (1.0 - step(0.0, delta.x));

    // Keypress rise: the glyph now occupying the previous cursor cell is
    // sampled from the live framebuffer, lifted, and settled into place.
    if (sameRow * oneRight > 0.5 && age < TYPE_DURATION) {
        float t = clamp(age / TYPE_DURATION, 0.0, 1.0);
        float settled = easeOutQuint(t);
        float lift = TYPE_RISE_PX * (1.0 - settled);
        vec2 cellHalf = previous.zw * 0.5;

        float originalCell = 1.0 - smoothstep(0.0, 1.25,
            sdBox(fragCoord, previousCenter, cellHalf));
        float originalGlyph = originalCell * foregroundMask(fragColor.rgb);
        fragColor.rgb = mix(fragColor.rgb, iBackgroundColor,
                            originalGlyph * (1.0 - settled));

        vec2 sampleCoord = fragCoord + vec2(0.0, lift);
        vec4 raised = texture(iChannel0, sampleCoord / iResolution.xy);
        float raisedCell = 1.0 - smoothstep(0.0, 1.25,
            sdBox(fragCoord, previousCenter - vec2(0.0, lift), cellHalf));
        float raisedGlyph = raisedCell * foregroundMask(raised.rgb);
        float arrival = smoothstep(0.0, 0.24, t);
        vec3 warmRaised = mix(raised.rgb, GOLD, 0.12 * (1.0 - t));
        fragColor.rgb = mix(fragColor.rgb, warmRaised, raisedGlyph * arrival);

        // Two faint echoes create the soft layered arrival Metalterm uses
        // without blurring every glyph on screen.
        for (int echo = 1; echo <= 2; ++echo) {
            float ef = float(echo);
            float echoLift = lift + ef * 2.2;
            vec4 echoSample = texture(iChannel0,
                (fragCoord + vec2(0.0, echoLift)) / iResolution.xy);
            float echoCell = 1.0 - smoothstep(0.0, 1.4,
                sdBox(fragCoord,
                      previousCenter - vec2(0.0, echoLift),
                      cellHalf));
            float echoGlyph = echoCell * foregroundMask(echoSample.rgb);
            float echoAlpha = echoGlyph * (1.0 - t) * (0.105 / ef);
            fragColor.rgb = mix(fragColor.rgb, mix(echoSample.rgb, GOLD, 0.22),
                                echoAlpha);
        }
    }

    // Backspace / one-cell-left decay. The deleted glyph is no longer present
    // in iChannel0, so the visible effect is a bright implosion followed by
    // fragments and dust emitted from the deleted cell.
    if (sameRow * oneLeft > 0.5 && age < DELETE_DURATION) {
        float t = clamp(age / DELETE_DURATION, 0.0, 1.0);
        float eased = easeOutQuint(t);
        float fade = 1.0 - smoothstep(0.42, 1.0, t);
        vec2 origin = currentCenter;

        float cellFlash = 1.0 - smoothstep(0.0, 0.22, t);
        float cell = 1.0 - smoothstep(0.0, 1.4,
            sdBox(fragCoord, origin, current.zw * 0.46));
        fragColor.rgb = mix(fragColor.rgb, mix(ICE, GOLD, 0.32),
                            cell * cellFlash * 0.34);

        float collapseRadius = mix(cellW * 0.58, 1.0, eased);
        float ring = circleMask(fragCoord, origin, collapseRadius)
            - circleMask(fragCoord, origin, max(collapseRadius - 1.7, 0.0));
        fragColor.rgb = mix(fragColor.rgb, ICE,
                            clamp(ring * fade * 0.72, 0.0, 0.72));

        float shards = 0.0;
        vec2 drift0 = vec2(-8.0,  8.0) * eased;
        vec2 drift1 = vec2( 7.0, 11.0) * eased;
        vec2 drift2 = vec2(-3.0, 15.0) * eased;
        vec2 drift3 = vec2(10.0,  4.0) * eased;
        shards += segmentMask(fragCoord, origin + vec2(-2.0,  1.0) + drift0,
                              origin + vec2( 0.5,  4.5) + drift0, 1.25);
        shards += segmentMask(fragCoord, origin + vec2( 1.0, -1.0) + drift1,
                              origin + vec2( 3.0,  2.0) + drift1, 1.10);
        shards += segmentMask(fragCoord, origin + vec2(-1.5,  0.0) + drift2,
                              origin + vec2(-0.5,  3.0) + drift2, 1.00);
        shards += segmentMask(fragCoord, origin + vec2( 2.0,  0.5) + drift3,
                              origin + vec2( 4.0, -1.0) + drift3, 1.05);

        float dust = 0.0;
        dust += circleMask(fragCoord, origin + vec2(-6.0,  5.0) * eased, 1.95);
        dust += circleMask(fragCoord, origin + vec2( 5.0,  8.0) * eased, 1.65);
        dust += circleMask(fragCoord, origin + vec2(-2.0, 12.0) * eased, 1.45);
        dust += circleMask(fragCoord, origin + vec2( 9.0,  2.0) * eased, 1.30);

        vec3 deleteColor = mix(ICE, GOLD, smoothstep(0.22, 0.75, t));
        fragColor.rgb = mix(fragColor.rgb, deleteColor,
                            clamp((shards * 0.68 + dust * 0.56) * fade, 0.0, 0.90));
    }

    // Enter / wrap settle: a warm highlight travels across the committed row.
    float rowMove = abs(delta.y);
    float changedOneRow = smoothstep(cellH * 0.48, cellH * 0.78, rowMove)
        * (1.0 - smoothstep(cellH * 1.7, cellH * 2.5, rowMove));
    float returnedLeft = 1.0 - step(-cellW * 0.5, delta.x);
    if (changedOneRow * returnedLeft > 0.5 && age < ENTER_DURATION) {
        float t = clamp(age / ENTER_DURATION, 0.0, 1.0);
        float sweepX = mix(-0.10 * iResolution.x,
                           1.10 * iResolution.x,
                           easeInOutCubic(t));
        float row = 1.0 - smoothstep(cellH * 0.46, cellH * 0.68,
                                     abs(fragCoord.y - previousCenter.y));
        float sweep = 1.0 - smoothstep(5.0, 34.0, abs(fragCoord.x - sweepX));
        float glyph = foregroundMask(fragColor.rgb);
        fragColor.rgb = mix(fragColor.rgb, GOLD,
                            row * glyph * sweep * (1.0 - t) * 0.30);
    }
}
