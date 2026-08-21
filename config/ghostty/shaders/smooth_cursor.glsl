// Smooth synthetic cursor for Ghostty 1.3+.
// The native cursor is made transparent in config; this shader draws the
// visible cursor at an interpolated position with a restrained elastic smear.
// Inspired by the geometry used by MIT-licensed Ghostty cursor shader projects,
// but deliberately tuned as a quiet, readable caret rather than a neon trail.

const float SHORT_DURATION = 0.24;
const float LONG_DURATION = 0.42;
const float TRAIL_OPACITY = 0.12;
const float CURSOR_OPACITY = 0.82;
const float GLOW_OPACITY = 0.24;
const float IDLE_BEFORE_BLINK = 0.85;
const float BLINK_PERIOD = 1.18;

vec3 sRGBToLinear(vec3 color) {
    return mix(color / 12.92,
               pow((color + 0.055) / 1.055, vec3(2.4)),
               step(vec3(0.04045), color));
}

vec2 cursorCenter(vec4 cursor) {
    return cursor.xy + vec2(cursor.z * 0.5, -cursor.w * 0.5);
}

float sdBox(vec2 point, vec2 center, vec2 halfSize) {
    vec2 d = abs(point - center) - halfSize;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}

float segmentDistance(vec2 point, vec2 start, vec2 end, out float along) {
    vec2 line = end - start;
    float denom = max(dot(line, line), 0.0001);
    along = clamp(dot(point - start, line) / denom, 0.0, 1.0);
    return length(point - (start + line * along));
}

float smootherstep01(float t) {
    t = clamp(t, 0.0, 1.0);
    return t * t * t * (t * (t * 6.0 - 15.0) + 10.0);
}

float softSpring(float t) {
    t = clamp(t, 0.0, 1.0);
    float settled = 1.0 - exp(-7.2 * t) * cos(9.2 * t);
    return t >= 0.999 ? 1.0 : settled;
}

float cursorBlink(float idle) {
    if (idle < IDLE_BEFORE_BLINK) {
        return 1.0;
    }
    float phase = (idle - IDLE_BEFORE_BLINK) / BLINK_PERIOD;
    float wave = 0.5 + 0.5 * cos(phase * 6.28318530718);
    return smoothstep(0.12, 0.88, wave);
}

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
    fragColor = texture(iChannel0, fragCoord / iResolution.xy);

    if (iFocus == 0 || iCursorVisible == 0) {
        return;
    }

    vec4 current = iCurrentCursor;
    vec4 previous = iPreviousCursor;
    vec2 currentCenter = cursorCenter(current);
    vec2 previousCenter = cursorCenter(previous);
    vec2 travel = currentCenter - previousCenter;
    float distancePx = length(travel);
    float cellScale = max(current.w, 1.0);
    float distanceCells = distancePx / cellScale;

    float duration = mix(SHORT_DURATION, LONG_DURATION,
                         smoothstep(1.0, 11.0, distanceCells));
    float age = max(iTime - iTimeCursorChange, 0.0);
    float raw = clamp(age / duration, 0.0, 1.0);
    float eased = softSpring(raw);

    // A small perpendicular arc prevents long diagonal jumps from looking
    // mechanical, while one-cell typing remains essentially straight.
    vec2 direction = distancePx > 0.1 ? travel / distancePx : vec2(1.0, 0.0);
    vec2 normal = vec2(-direction.y, direction.x);
    float arcStrength = min(distancePx * 0.055, current.w * 0.72);
    float arc = sin(3.14159265359 * clamp(raw, 0.0, 1.0));
    vec2 animatedCenter = mix(previousCenter, currentCenter, eased)
        + normal * arcStrength * arc;

    vec2 halfSize = current.zw * 0.5;
    if (iCurrentCursorStyle == CURSORSTYLE_BAR) {
        halfSize.x = max(1.35, current.z * 0.12);
        animatedCenter.x -= current.z * 0.5 - halfSize.x;
    } else if (iCurrentCursorStyle == CURSORSTYLE_UNDERLINE) {
        halfSize.y = max(1.25, current.w * 0.09);
        animatedCenter.y -= current.w * 0.5 - halfSize.y;
    } else if (iCurrentCursorStyle == CURSORSTYLE_BLOCK_HOLLOW) {
        halfSize *= 0.92;
    }

    vec3 cursorColor = sRGBToLinear(iCursorColor);
    float aa = 1.25;
    float cursorSd = sdBox(fragCoord, animatedCenter, halfSize);
    float cursorMask = 1.0 - smoothstep(-0.4, aa, cursorSd);

    if (iCurrentCursorStyle == CURSORSTYLE_BLOCK_HOLLOW) {
        float inner = 1.0 - smoothstep(-0.4, aa,
            sdBox(fragCoord, animatedCenter, max(halfSize - vec2(1.6), vec2(0.5))));
        cursorMask = max(cursorMask - inner, 0.0);
    }

    float motion = 1.0 - smootherstep01(raw);
    float along = 0.0;
    float trailDistance = segmentDistance(fragCoord, previousCenter,
                                          animatedCenter, along);
    float trailWidth = mix(max(1.2, min(halfSize.x, halfSize.y) * 0.20),
                           max(1.8, min(halfSize.x, halfSize.y) * 0.42),
                           along);
    float trail = 1.0 - smoothstep(trailWidth, trailWidth + 2.4, trailDistance);
    trail *= smoothstep(0.0, 0.16, along) * motion;

    // A broad, low-opacity halo gives the cursor weight without turning the
    // terminal into a bloom demo.
    float glow = 1.0 - smoothstep(1.5, max(current.w * 0.72, 6.0),
                                  max(cursorSd, 0.0));
    glow *= GLOW_OPACITY * (0.72 + motion * 0.28);

    fragColor.rgb = mix(fragColor.rgb, cursorColor,
                        clamp(trail * TRAIL_OPACITY, 0.0, TRAIL_OPACITY));
    fragColor.rgb = mix(fragColor.rgb, cursorColor,
                        clamp(glow, 0.0, GLOW_OPACITY));

    float blink = cursorBlink(age);
    float opacity = CURSOR_OPACITY * blink;
    if (iCurrentCursorStyle == CURSORSTYLE_BAR
        || iCurrentCursorStyle == CURSORSTYLE_UNDERLINE) {
        opacity = min(opacity + 0.12, 0.96);
    }
    fragColor.rgb = mix(fragColor.rgb, cursorColor,
                        clamp(cursorMask * opacity, 0.0, 0.96));

    // Thin highlight on the leading edge: this is what makes the block read as
    // a moving object rather than just a fading rectangle.
    float leading = dot(fragCoord - animatedCenter, direction);
    float rim = cursorMask
        * smoothstep(max(halfSize.x, halfSize.y) * 0.08,
                     max(halfSize.x, halfSize.y) * 0.72,
                     leading);
    fragColor.rgb = mix(fragColor.rgb, vec3(1.0), rim * motion * 0.08);
}
