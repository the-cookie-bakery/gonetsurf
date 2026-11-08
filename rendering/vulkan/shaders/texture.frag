#version 450

layout(set = 0, binding = 0) uniform sampler2D sourceTexture;

layout(push_constant) uniform PushConstants {
    vec4 rect;
    vec4 uv;
    vec4 color;
    vec4 params;
} push;

layout(location = 0) in vec4 fragmentColor;
layout(location = 1) in vec2 fragmentUV;
layout(location = 0) out vec4 outColor;

void main() {
    vec4 sampleColor = texture(sourceTexture, fragmentUV);
    if (push.params.y < 0.5) {
        float alpha = fragmentColor.a * push.params.x;
        outColor = vec4(fragmentColor.rgb * alpha, alpha);
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
