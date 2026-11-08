//go:build vulkan && cgo

#ifndef GONETSURF_VK_SHARED_H
#define GONETSURF_VK_SHARED_H

#include <stdint.h>
#include <stddef.h>

int vk_shared_init(char *error, size_t error_size);
uintptr_t vk_shared_instance(void);
uintptr_t vk_shared_physical(void);
uintptr_t vk_shared_device(void);
uintptr_t vk_shared_queue(void);
uint32_t vk_shared_queue_family(void);

#endif
