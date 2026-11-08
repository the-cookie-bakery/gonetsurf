#version 450

layout(set = 0, binding = 0) uniform sampler2D sourceTexture;
layout(set = 0, binding = 1) uniform sampler2D maskTexture;

layout(push_constant) uniform PushConstants {
    vec4 rect;
    vec4 uv;
    vec4 color;
    vec4 params;
    vec4 transform0;
    vec4 transform1;
} push;

layout(location = 0) in vec4 fragmentColor;
layout(location = 1) in vec2 fragmentUV;
layout(location = 0) out vec4 outColor;

float gradientData(float index, float width, float height) {
    vec4 texel = texture(sourceTexture, vec2((index + 0.5) / width, 0.5 / height));
    uvec4 bytes = uvec4(round(texel * 255.0));
    uint bits = bytes.r | (bytes.g << 8) | (bytes.b << 16) | (bytes.a << 24);
    return uintBitsToFloat(bits);
}

vec4 sampleGradient(float position, float stopCount, float width, float height) {
    if (stopCount < 0.5) {
        return vec4(0.0);
    }
    float firstPosition = gradientData(18.0, width, height);
    vec4 firstColor = vec4(
        gradientData(19.0, width, height),
        gradientData(20.0, width, height),
        gradientData(21.0, width, height),
        gradientData(22.0, width, height)) / 255.0;
    if (stopCount < 1.5) {
        return firstColor;
    }
    float lastPosition = gradientData(18.0 + (stopCount - 1.0) * 5.0, width, height);
    vec4 lastColor = vec4(
        gradientData(19.0 + (stopCount - 1.0) * 5.0, width, height),
        gradientData(20.0 + (stopCount - 1.0) * 5.0, width, height),
        gradientData(21.0 + (stopCount - 1.0) * 5.0, width, height),
        gradientData(22.0 + (stopCount - 1.0) * 5.0, width, height)) / 255.0;
    if (position <= firstPosition) {
        return firstColor;
    }
    if (position >= lastPosition) {
        return lastColor;
    }
    for (int index = 0; index < 64; index++) {
        if (float(index) + 1.0 >= stopCount) {
            break;
        }
        float base = 18.0 + float(index) * 5.0;
        float startPosition = gradientData(base, width, height);
        vec4 startColor = vec4(
            gradientData(base + 1.0, width, height),
            gradientData(base + 2.0, width, height),
            gradientData(base + 3.0, width, height),
            gradientData(base + 4.0, width, height)) / 255.0;
        float nextBase = base + 5.0;
        float endPosition = gradientData(nextBase, width, height);
        if (position < startPosition || position > endPosition) {
            continue;
        }
        float span = endPosition - startPosition;
        if (span <= 0.0) {
            return vec4(
                gradientData(nextBase + 1.0, width, height),
                gradientData(nextBase + 2.0, width, height),
                gradientData(nextBase + 3.0, width, height),
                gradientData(nextBase + 4.0, width, height)) / 255.0;
        }
        float fraction = (position - startPosition) / span;
        vec4 endColor = vec4(
            gradientData(nextBase + 1.0, width, height),
            gradientData(nextBase + 2.0, width, height),
            gradientData(nextBase + 3.0, width, height),
            gradientData(nextBase + 4.0, width, height)) / 255.0;
        float startAlpha = startColor.a;
        float endAlpha = endColor.a;
        float alpha = startAlpha * (1.0 - fraction) + endAlpha * fraction;
        if (alpha <= 0.0) {
            return vec4(0.0);
        }
        vec3 rgb = (startColor.rgb * startAlpha * (1.0 - fraction) + endColor.rgb * endAlpha * fraction) / alpha;
        return vec4(rgb, alpha);
    }
    return lastColor;
}

void main() {
    if (push.params.y > 3.5 && push.params.y < 4.5) {
        float width = max(1.0, fragmentUV.x);
        float textureHeight = max(1.0, fragmentUV.y);
        float kind = gradientData(0.0, width, textureHeight);
        float repeating = gradientData(1.0, width, textureHeight);
        float stopCount = gradientData(2.0, width, textureHeight);
        float maskEnabled = gradientData(3.0, width, textureHeight);
        float maskMinimumX = gradientData(6.0, width, textureHeight);
        float maskMinimumY = gradientData(7.0, width, textureHeight);
        float maskWidth = gradientData(8.0, width, textureHeight);
        float maskHeight = gradientData(9.0, width, textureHeight);
        float centerX = gradientData(10.0, width, textureHeight);
        float centerY = gradientData(11.0, width, textureHeight);
        float directionX = gradientData(12.0, width, textureHeight);
        float directionY = gradientData(13.0, width, textureHeight);
        float lineLength = gradientData(14.0, width, textureHeight);
        float radiusX = gradientData(15.0, width, textureHeight);
        float radiusY = gradientData(16.0, width, textureHeight);
        float fromAngle = gradientData(17.0, width, textureHeight);
        vec2 position = gl_FragCoord.xy;
        float value = 0.0;
        if (kind < 0.5) {
            if (lineLength > 0.0) {
                float offset = (position.x - centerX) * directionX + (position.y - centerY) * directionY;
                value = 0.5 + offset / lineLength;
            }
        } else if (kind < 1.5) {
            if (radiusX > 0.0 && radiusY > 0.0) {
                vec2 normalized = vec2((position.x - centerX) / radiusX, (position.y - centerY) / radiusY);
                value = length(normalized);
            }
        } else {
            float angle = atan(position.x - centerX, -(position.y - centerY));
            if (angle < 0.0) {
                angle += 6.283185307179586;
            }
            value = (angle - fromAngle) / 6.283185307179586;
            value = value - floor(value);
        }
        if (repeating > 0.5 && stopCount > 1.5) {
            float firstPosition = gradientData(18.0, width, textureHeight);
            float lastPosition = gradientData(18.0 + (stopCount - 1.0) * 5.0, width, textureHeight);
            float period = lastPosition - firstPosition;
            if (period <= 0.0) {
                period = 1.0;
            }
            float wrapped = mod(value - firstPosition, period);
            if (wrapped < 0.0) {
                wrapped += period;
            }
            value = firstPosition + wrapped;
        }
        vec4 sampleColor = sampleGradient(value, stopCount, width, textureHeight);
        float coverage = 1.0;
        if (maskEnabled > 0.5) {
            vec2 local = floor(gl_FragCoord.xy - vec2(maskMinimumX, maskMinimumY));
            if (local.x < 0.0 || local.y < 0.0 || local.x >= maskWidth || local.y >= maskHeight) {
                coverage = 0.0;
            } else {
                coverage = texture(sourceTexture, vec2((local.x + 0.5) / width, (1.0 + local.y + 0.5) / textureHeight)).r;
            }
        }
        float alpha = sampleColor.a * push.params.x * coverage;
        outColor = vec4(sampleColor.rgb * alpha, alpha);
        return;
    }
    vec4 sampleColor = texture(sourceTexture, fragmentUV);
    if (push.params.y > 5.5) {
        float alpha = fragmentColor.a * push.params.x;
        outColor = vec4(fragmentColor.rgb * alpha, alpha);
    } else if (push.params.y < 0.5) {
        float alpha = fragmentColor.a * push.params.x;
        outColor = vec4(fragmentColor.rgb * alpha, alpha);
    } else if (push.params.y > 4.5) {
        vec4 layerColor = texture(sourceTexture, fragmentUV);
        float maskCoverage = texture(maskTexture, fragmentUV).r;
        outColor = layerColor * maskCoverage * push.params.x;
    } else if (push.params.y > 2.5) {
        outColor = sampleColor * push.params.x;
    } else if (push.params.y > 1.5) {
        float coverage = sampleColor.r;
        float alpha = fragmentColor.a * coverage * push.params.x;
        outColor = vec4(fragmentColor.rgb * alpha, alpha);
    } else {
        vec4 premultiplied = vec4(sampleColor.rgb * sampleColor.a, sampleColor.a);
        outColor = premultiplied * push.params.x;
    }
}
