#ifndef GONETSURF_VK_HELPER_H
#define GONETSURF_VK_HELPER_H

#include <stdint.h>
#include <stddef.h>

typedef struct {
    float rect[4];
    float uv[4];
    float color[4];
    float clip[4];
    float opacity;
    int mode;
    uint64_t texture;
} vk_draw;

typedef struct vk_context vk_context;

vk_context *vk_context_create(int width, int height,
    const uint32_t *vertex_code, size_t vertex_size,
    const uint32_t *solid_code, size_t solid_size,
    const uint32_t *texture_code, size_t texture_size,
    char *error, size_t error_size);
int vk_context_create_texture(vk_context *context, const uint8_t *pixels, int width, int height, int format, char *error, size_t error_size);
int vk_context_render(vk_context *context, const float clear[4], const vk_draw *draws, int draw_count, uint8_t *out_pixels, char *error, size_t error_size);
void vk_context_destroy(vk_context *context);

#endif
