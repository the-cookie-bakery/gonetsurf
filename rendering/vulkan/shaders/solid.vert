#version 450

layout(push_constant) uniform PushConstants {
    vec2 position;
    vec2 size;
    vec4 color;
} constants;

layout(location = 0) out vec4 fragmentColor;

void main() {
    vec2 corner = vec2(float((gl_VertexIndex << 1) & 2), float(gl_VertexIndex & 2));
    vec2 position = constants.position + corner * constants.size;
    vec2 clip = vec2(position.x * 2.0 - 1.0, 1.0 - position.y * 2.0);
    gl_Position = vec4(clip, 0.0, 1.0);
    fragmentColor = constants.color;
}
