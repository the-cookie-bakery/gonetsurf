#version 450

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
    float alpha = fragmentColor.a * push.params.x;
    outColor = vec4(fragmentColor.rgb * alpha, alpha);
}
