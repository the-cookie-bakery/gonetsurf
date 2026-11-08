#version 450

layout(std430, binding = 2) buffer Nodes {
    float nodes[];
};

layout(push_constant) uniform PushConstants {
    vec4 rect;
    vec4 uv;
    vec4 color;
    vec4 params;
    vec4 transform0;
    vec4 transform1;
} push;

layout(location = 0) out vec4 fragmentColor;
layout(location = 1) out vec2 fragmentUV;

void main() {
    vec2 corner = vec2(float((gl_VertexIndex << 1) & 2), float(gl_VertexIndex & 2)) * 0.5;
    vec2 pixel;
    if (push.params.y > 5.5) {
        int node = int(push.transform0.x + 0.5);
        int stride = int(push.transform0.y + 0.5);
        int xField = int(push.transform0.z + 0.5);
        int yField = int(push.transform0.w + 0.5);
        int widthField = int(push.transform1.x + 0.5);
        int heightField = int(push.transform1.y + 0.5);
        vec2 origin = vec2(nodes[node * stride + xField], nodes[node * stride + yField]);
        vec2 size = vec2(nodes[node * stride + widthField], nodes[node * stride + heightField]);
        pixel = origin + corner * size;
    } else {
        pixel = push.rect.xy + corner * push.rect.zw;
        if (push.transform1.z > 0.5) {
            vec2 transformed = vec2(
                push.transform0.x * pixel.x + push.transform0.z * pixel.y + push.transform1.x,
                push.transform0.y * pixel.x + push.transform0.w * pixel.y + push.transform1.y);
            pixel = transformed;
        }
    }
    vec2 clip = vec2(pixel.x / push.params.z * 2.0 - 1.0, pixel.y / push.params.w * 2.0 - 1.0);
    gl_Position = vec4(clip, 0.0, 1.0);
    fragmentColor = push.color;
    fragmentUV = mix(push.uv.xy, push.uv.zw, corner);
}
