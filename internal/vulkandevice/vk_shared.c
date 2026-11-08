//go:build vulkan && cgo

#include "vk_shared.h"
#include <vulkan/vulkan.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

typedef struct {
    int initialized;
    VkInstance instance;
    VkPhysicalDevice physical;
    VkDevice device;
    VkQueue queue;
    uint32_t queue_family;
} vk_shared_state;

static vk_shared_state shared;

static void set_error(char *error, size_t error_size, const char *message) {
    if (error != NULL && error_size > 0) {
        snprintf(error, error_size, "%s", message);
    }
}

int vk_shared_init(char *error, size_t error_size) {
    if (shared.initialized) {
        return 1;
    }
    VkApplicationInfo application = {0};
    application.sType = VK_STRUCTURE_TYPE_APPLICATION_INFO;
    application.pApplicationName = "gonetsurf";
    application.apiVersion = VK_API_VERSION_1_1;
    const char *wanted_extensions[2] = {"VK_KHR_surface", "VK_KHR_xcb_surface"};
    const char *enabled_extensions[2];
    uint32_t enabled_count = 0;
    uint32_t extension_count = 0;
    vkEnumerateInstanceExtensionProperties(NULL, &extension_count, NULL);
    VkExtensionProperties *extensions = calloc(extension_count, sizeof(VkExtensionProperties));
    if (extensions != NULL) {
        vkEnumerateInstanceExtensionProperties(NULL, &extension_count, extensions);
    }
    for (int wanted = 0; wanted < 2; wanted++) {
        for (uint32_t index = 0; index < extension_count; index++) {
            if (strcmp(extensions[index].extensionName, wanted_extensions[wanted]) == 0) {
                enabled_extensions[enabled_count++] = wanted_extensions[wanted];
                break;
            }
        }
    }
    free(extensions);
    VkInstanceCreateInfo instance_info = {0};
    instance_info.sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO;
    instance_info.pApplicationInfo = &application;
    instance_info.enabledExtensionCount = enabled_count;
    instance_info.ppEnabledExtensionNames = enabled_extensions;
    if (vkCreateInstance(&instance_info, NULL, &shared.instance) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create shared Vulkan instance");
        return 0;
    }
    uint32_t device_count = 0;
    vkEnumeratePhysicalDevices(shared.instance, &device_count, NULL);
    if (device_count == 0) {
        set_error(error, error_size, "no Vulkan devices");
        return 0;
    }
    VkPhysicalDevice *devices = calloc(device_count, sizeof(VkPhysicalDevice));
    if (devices == NULL) {
        set_error(error, error_size, "out of memory");
        return 0;
    }
    vkEnumeratePhysicalDevices(shared.instance, &device_count, devices);
    int found = 0;
    for (uint32_t device_index = 0; device_index < device_count && !found; device_index++) {
        VkPhysicalDevice physical = devices[device_index];
        uint32_t family_count = 0;
        vkGetPhysicalDeviceQueueFamilyProperties(physical, &family_count, NULL);
        VkQueueFamilyProperties *families = calloc(family_count, sizeof(VkQueueFamilyProperties));
        if (families == NULL) {
            continue;
        }
        vkGetPhysicalDeviceQueueFamilyProperties(physical, &family_count, families);
        for (uint32_t family = 0; family < family_count; family++) {
            VkQueueFlags required = VK_QUEUE_GRAPHICS_BIT | VK_QUEUE_COMPUTE_BIT;
            if ((families[family].queueFlags & required) == required) {
                shared.physical = physical;
                shared.queue_family = family;
                found = 1;
                break;
            }
        }
        free(families);
    }
    free(devices);
    if (!found) {
        set_error(error, error_size, "no graphics+compute queue family");
        return 0;
    }
    const char *device_extensions[1] = {"VK_KHR_swapchain"};
    uint32_t device_extension_count = 0;
    vkEnumerateDeviceExtensionProperties(shared.physical, NULL, &device_extension_count, NULL);
    VkExtensionProperties *device_extensions_available = calloc(device_extension_count, sizeof(VkExtensionProperties));
    if (device_extensions_available != NULL) {
        vkEnumerateDeviceExtensionProperties(shared.physical, NULL, &device_extension_count, device_extensions_available);
    }
    const char *enabled_device_extensions[1];
    uint32_t enabled_device_count = 0;
    for (uint32_t index = 0; index < device_extension_count; index++) {
        if (strcmp(device_extensions_available[index].extensionName, device_extensions[0]) == 0) {
            enabled_device_extensions[enabled_device_count++] = device_extensions[0];
            break;
        }
    }
    free(device_extensions_available);

    float priority = 1.0f;
    VkDeviceQueueCreateInfo queue_info = {0};
    queue_info.sType = VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO;
    queue_info.queueFamilyIndex = shared.queue_family;
    queue_info.queueCount = 1;
    queue_info.pQueuePriorities = &priority;
    VkDeviceCreateInfo device_info = {0};
    device_info.sType = VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO;
    device_info.queueCreateInfoCount = 1;
    device_info.pQueueCreateInfos = &queue_info;
    device_info.enabledExtensionCount = enabled_device_count;
    device_info.ppEnabledExtensionNames = enabled_device_extensions;
    if (vkCreateDevice(shared.physical, &device_info, NULL, &shared.device) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create shared Vulkan device");
        return 0;
    }
    vkGetDeviceQueue(shared.device, shared.queue_family, 0, &shared.queue);
    shared.initialized = 1;
    return 1;
}

uintptr_t vk_shared_instance(void) { return (uintptr_t)shared.instance; }
uintptr_t vk_shared_physical(void) { return (uintptr_t)shared.physical; }
uintptr_t vk_shared_device(void) { return (uintptr_t)shared.device; }
uintptr_t vk_shared_queue(void) { return (uintptr_t)shared.queue; }
uint32_t vk_shared_queue_family(void) { return shared.queue_family; }
