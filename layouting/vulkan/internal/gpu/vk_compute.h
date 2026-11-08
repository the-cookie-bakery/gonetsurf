#ifndef GONETSURF_VK_COMPUTE_H
#define GONETSURF_VK_COMPUTE_H

#include <stdint.h>
#include <stddef.h>

typedef struct vk_compute_context vk_compute_context;
typedef struct vk_compute_pipeline vk_compute_pipeline;

vk_compute_context *vk_compute_create(int buffer_count, uint32_t push_size,
    uintptr_t instance_handle, uintptr_t physical_handle, uintptr_t device_handle,
    uintptr_t queue_handle, uint32_t queue_family,
    char *error, size_t error_size);
int vk_compute_buffer(vk_compute_context *context, int index, size_t size,
    char *error, size_t error_size);
int vk_compute_upload(vk_compute_context *context, int index,
    const void *data, size_t size);
int vk_compute_upload_range(vk_compute_context *context, int index,
    size_t offset, const void *data, size_t size);
int vk_compute_readback(vk_compute_context *context, int index,
    void *data, size_t size);
vk_compute_pipeline *vk_compute_pipeline_create(vk_compute_context *context,
    const uint32_t *code, size_t size, char *error, size_t error_size);
int vk_compute_begin(vk_compute_context *context, char *error, size_t error_size);
int vk_compute_record(vk_compute_context *context, vk_compute_pipeline *pipeline,
    uint32_t groups_x, uint32_t groups_y, const void *push, size_t push_size,
    char *error, size_t error_size);
int vk_compute_end(vk_compute_context *context, char *error, size_t error_size);
int vk_compute_dispatch(vk_compute_context *context, vk_compute_pipeline *pipeline,
    uint32_t groups_x, uint32_t groups_y, const void *push, size_t push_size,
    char *error, size_t error_size);
uint64_t vk_compute_buffer_handle(vk_compute_context *context, int index);
void vk_compute_pipeline_destroy(vk_compute_context *context,
    vk_compute_pipeline *pipeline);
void vk_compute_destroy(vk_compute_context *context);

#endif
