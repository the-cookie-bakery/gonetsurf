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
    float transform[6];
    int transformed;
} vk_draw;

typedef struct vk_context vk_context;

vk_context *vk_context_create(int width, int height,
    const uint32_t *vertex_code, size_t vertex_size,
    const uint32_t *solid_code, size_t solid_size,
    const uint32_t *texture_code, size_t texture_size,
    uintptr_t instance_handle, uintptr_t physical_handle, uintptr_t device_handle,
    uintptr_t queue_handle, uint32_t queue_family,
    char *error, size_t error_size);
int vk_context_create_texture(vk_context *context, const uint8_t *pixels, int width, int height, int format, char *error, size_t error_size);
int vk_context_render(vk_context *context, const float clear[4], const vk_draw *draws, int draw_count, uint8_t *out_pixels, char *error, size_t error_size);
int vk_context_geometry_texture(vk_context *context, uintptr_t buffer, char *error, size_t error_size);
int vk_context_attach_swapchain(vk_context *context, const char *display, uintptr_t window,
    int width, int height, char *error, size_t error_size);
int vk_context_present_surface(vk_context *context, const uint8_t *pixels,
    int width, int height, char *error, size_t error_size);
int vk_context_create_path_pipeline(vk_context *context, const uint32_t *code, size_t size, char *error, size_t error_size);
int vk_context_rasterize_path(vk_context *context, const float *edges, int edge_count,
    int width, int height, int rule, char *error, size_t error_size);
int vk_context_begin_layers(vk_context *context, char *error, size_t error_size);
int vk_context_render_layer(vk_context *context, int width, int height,
    const float clear[4], const vk_draw *draws, int draw_count,
    char *error, size_t error_size);
int vk_context_pair_texture(vk_context *context, int first, int second,
    char *error, size_t error_size);
void vk_context_destroy(vk_context *context);

#endif
