#version 450

layout(push_constant) uniform PushConstants {
    vec4 rect;
    vec4 uv;
    vec4 color;
    vec4 params;
} push;

layout(location = 0) out vec4 fragmentColor;
layout(location = 1) out vec2 fragmentUV;

void main() {
    vec2 corner = vec2(float((gl_VertexIndex << 1) & 2), float(gl_VertexIndex & 2)) * 0.5;
    vec2 pixel = push.rect.xy + corner * push.rect.zw;
    vec2 clip = vec2(pixel.x / push.params.z * 2.0 - 1.0, pixel.y / push.params.w * 2.0 - 1.0);
    gl_Position = vec4(clip, 0.0, 1.0);
    fragmentColor = push.color;
    fragmentUV = mix(push.uv.xy, push.uv.zw, corner);
}
