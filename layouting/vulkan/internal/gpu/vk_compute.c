#include "vk_compute.h"
#include <vulkan/vulkan.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

#define MAX_BUFFERS 16

typedef struct {
    VkBuffer buffer;
    VkDeviceMemory memory;
    size_t size;
} vk_buffer_state;

struct vk_compute_pipeline {
    VkShaderModule module;
    VkPipeline pipeline;
};

struct vk_compute_context {
    VkInstance instance;
    VkPhysicalDevice physical;
    VkDevice device;
    VkQueue queue;
    uint32_t queue_family;
    VkCommandPool command_pool;
    VkCommandBuffer command;
    VkFence fence;
    VkDescriptorSetLayout set_layout;
    VkDescriptorPool descriptor_pool;
    VkDescriptorSet descriptor_set;
    VkPipelineLayout pipeline_layout;
    int owns_device;
    int buffer_count;
    int recording;
    vk_buffer_state buffers[MAX_BUFFERS];
};

static void set_error(char *error, size_t error_size, const char *message) {
    if (error != NULL && error_size > 0) {
        snprintf(error, error_size, "%s", message);
    }
}

static uint32_t find_memory_type(vk_compute_context *context, uint32_t bits,
    VkMemoryPropertyFlags properties) {
    VkPhysicalDeviceMemoryProperties memory_properties;
    vkGetPhysicalDeviceMemoryProperties(context->physical, &memory_properties);
    for (uint32_t index = 0; index < memory_properties.memoryTypeCount; index++) {
        if ((bits & (1u << index)) &&
            (memory_properties.memoryTypes[index].propertyFlags & properties) == properties) {
            return index;
        }
    }
    return UINT32_MAX;
}

vk_compute_context *vk_compute_create(int buffer_count, uint32_t push_size,
    uintptr_t instance_handle, uintptr_t physical_handle, uintptr_t device_handle,
    uintptr_t queue_handle, uint32_t queue_family,
    char *error, size_t error_size) {
    if (buffer_count <= 0 || buffer_count > MAX_BUFFERS) {
        set_error(error, error_size, "invalid buffer count");
        return NULL;
    }
    vk_compute_context *context = calloc(1, sizeof(vk_compute_context));
    if (context == NULL) {
        set_error(error, error_size, "out of memory");
        return NULL;
    }
    context->buffer_count = buffer_count;

    if (device_handle != 0) {
        context->instance = (VkInstance)instance_handle;
        context->physical = (VkPhysicalDevice)physical_handle;
        context->device = (VkDevice)device_handle;
        context->queue = (VkQueue)queue_handle;
        context->queue_family = queue_family;
        context->owns_device = 0;
    } else {
    VkApplicationInfo application = {0};
    application.sType = VK_STRUCTURE_TYPE_APPLICATION_INFO;
    application.pApplicationName = "gonetsurf-layout";
    application.apiVersion = VK_API_VERSION_1_1;
    VkInstanceCreateInfo instance_info = {0};
    instance_info.sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO;
    instance_info.pApplicationInfo = &application;
    if (vkCreateInstance(&instance_info, NULL, &context->instance) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create Vulkan instance");
        free(context);
        return NULL;
    }

    uint32_t device_count = 0;
    vkEnumeratePhysicalDevices(context->instance, &device_count, NULL);
    if (device_count == 0) {
        set_error(error, error_size, "no Vulkan devices");
        vkDestroyInstance(context->instance, NULL);
        free(context);
        return NULL;
    }
    VkPhysicalDevice *devices = calloc(device_count, sizeof(VkPhysicalDevice));
    vkEnumeratePhysicalDevices(context->instance, &device_count, devices);
    context->physical = devices[0];
    free(devices);

    uint32_t family_count = 0;
    vkGetPhysicalDeviceQueueFamilyProperties(context->physical, &family_count, NULL);
    VkQueueFamilyProperties *families = calloc(family_count, sizeof(VkQueueFamilyProperties));
    vkGetPhysicalDeviceQueueFamilyProperties(context->physical, &family_count, families);
    int found = 0;
    for (uint32_t index = 0; index < family_count; index++) {
        if (families[index].queueFlags & VK_QUEUE_COMPUTE_BIT) {
            context->queue_family = index;
            found = 1;
            break;
        }
    }
    free(families);
    if (!found) {
        set_error(error, error_size, "no compute queue family");
        vkDestroyInstance(context->instance, NULL);
        free(context);
        return NULL;
    }

    float priority = 1.0f;
    VkDeviceQueueCreateInfo queue_info = {0};
    queue_info.sType = VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO;
    queue_info.queueFamilyIndex = context->queue_family;
    queue_info.queueCount = 1;
    queue_info.pQueuePriorities = &priority;
    VkDeviceCreateInfo device_info = {0};
    device_info.sType = VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO;
    device_info.queueCreateInfoCount = 1;
    device_info.pQueueCreateInfos = &queue_info;
    if (vkCreateDevice(context->physical, &device_info, NULL, &context->device) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create Vulkan device");
        vkDestroyInstance(context->instance, NULL);
        free(context);
        return NULL;
    }
    vkGetDeviceQueue(context->device, context->queue_family, 0, &context->queue);
    context->owns_device = 1;
    }

    VkCommandPoolCreateInfo pool_info = {0};
    pool_info.sType = VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO;
    pool_info.flags = VK_COMMAND_POOL_CREATE_RESET_COMMAND_BUFFER_BIT;
    pool_info.queueFamilyIndex = context->queue_family;
    if (vkCreateCommandPool(context->device, &pool_info, NULL, &context->command_pool) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create command pool");
        vk_compute_destroy(context);
        return NULL;
    }
    VkCommandBufferAllocateInfo allocate_info = {0};
    allocate_info.sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO;
    allocate_info.commandPool = context->command_pool;
    allocate_info.level = VK_COMMAND_BUFFER_LEVEL_PRIMARY;
    allocate_info.commandBufferCount = 1;
    if (vkAllocateCommandBuffers(context->device, &allocate_info, &context->command) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate command buffer");
        vk_compute_destroy(context);
        return NULL;
    }
    VkFenceCreateInfo fence_info = {0};
    fence_info.sType = VK_STRUCTURE_TYPE_FENCE_CREATE_INFO;
    if (vkCreateFence(context->device, &fence_info, NULL, &context->fence) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create fence");
        vk_compute_destroy(context);
        return NULL;
    }

    VkDescriptorSetLayoutBinding *bindings =
        calloc(buffer_count, sizeof(VkDescriptorSetLayoutBinding));
    for (int index = 0; index < buffer_count; index++) {
        bindings[index].binding = (uint32_t)index;
        bindings[index].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
        bindings[index].descriptorCount = 1;
        bindings[index].stageFlags = VK_SHADER_STAGE_COMPUTE_BIT;
    }
    VkDescriptorSetLayoutCreateInfo layout_info = {0};
    layout_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO;
    layout_info.bindingCount = (uint32_t)buffer_count;
    layout_info.pBindings = bindings;
    VkResult layout_result =
        vkCreateDescriptorSetLayout(context->device, &layout_info, NULL, &context->set_layout);
    free(bindings);
    if (layout_result != VK_SUCCESS) {
        set_error(error, error_size, "failed to create descriptor set layout");
        vk_compute_destroy(context);
        return NULL;
    }

    VkDescriptorPoolSize pool_size = {0};
    pool_size.type = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    pool_size.descriptorCount = (uint32_t)buffer_count;
    VkDescriptorPoolCreateInfo descriptor_pool_info = {0};
    descriptor_pool_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO;
    descriptor_pool_info.maxSets = 1;
    descriptor_pool_info.poolSizeCount = 1;
    descriptor_pool_info.pPoolSizes = &pool_size;
    if (vkCreateDescriptorPool(context->device, &descriptor_pool_info, NULL,
            &context->descriptor_pool) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create descriptor pool");
        vk_compute_destroy(context);
        return NULL;
    }
    VkDescriptorSetAllocateInfo set_allocate = {0};
    set_allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
    set_allocate.descriptorPool = context->descriptor_pool;
    set_allocate.descriptorSetCount = 1;
    set_allocate.pSetLayouts = &context->set_layout;
    if (vkAllocateDescriptorSets(context->device, &set_allocate,
            &context->descriptor_set) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate descriptor set");
        vk_compute_destroy(context);
        return NULL;
    }

    VkPushConstantRange range = {0};
    range.stageFlags = VK_SHADER_STAGE_COMPUTE_BIT;
    range.offset = 0;
    range.size = push_size;
    VkPipelineLayoutCreateInfo pipeline_layout_info = {0};
    pipeline_layout_info.sType = VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO;
    pipeline_layout_info.setLayoutCount = 1;
    pipeline_layout_info.pSetLayouts = &context->set_layout;
    pipeline_layout_info.pushConstantRangeCount = push_size > 0 ? 1 : 0;
    pipeline_layout_info.pPushConstantRanges = &range;
    if (vkCreatePipelineLayout(context->device, &pipeline_layout_info, NULL,
            &context->pipeline_layout) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create pipeline layout");
        vk_compute_destroy(context);
        return NULL;
    }
    return context;
}

int vk_compute_buffer(vk_compute_context *context, int index, size_t size,
    char *error, size_t error_size) {
    if (context == NULL || index < 0 || index >= context->buffer_count) {
        set_error(error, error_size, "invalid buffer index");
        return 0;
    }
    if (size < 4) {
        size = 4;
    }
    if (context->buffers[index].buffer != VK_NULL_HANDLE) {
        vkDestroyBuffer(context->device, context->buffers[index].buffer, NULL);
        vkFreeMemory(context->device, context->buffers[index].memory, NULL);
    }
    VkBufferCreateInfo info = {0};
    info.sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO;
    info.size = size;
    info.usage = VK_BUFFER_USAGE_STORAGE_BUFFER_BIT;
    info.sharingMode = VK_SHARING_MODE_EXCLUSIVE;
    if (vkCreateBuffer(context->device, &info, NULL,
            &context->buffers[index].buffer) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create storage buffer");
        return 0;
    }
    VkMemoryRequirements requirements;
    vkGetBufferMemoryRequirements(context->device, context->buffers[index].buffer,
        &requirements);
    VkMemoryAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO;
    allocate.allocationSize = requirements.size;
    allocate.memoryTypeIndex = find_memory_type(context, requirements.memoryTypeBits,
        VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT);
    if (allocate.memoryTypeIndex == UINT32_MAX ||
        vkAllocateMemory(context->device, &allocate, NULL,
            &context->buffers[index].memory) != VK_SUCCESS ||
        vkBindBufferMemory(context->device, context->buffers[index].buffer,
            context->buffers[index].memory, 0) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate buffer memory");
        return 0;
    }
    context->buffers[index].size = size;

    VkDescriptorBufferInfo buffer_info = {0};
    buffer_info.buffer = context->buffers[index].buffer;
    buffer_info.offset = 0;
    buffer_info.range = size;
    VkWriteDescriptorSet write = {0};
    write.sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
    write.dstSet = context->descriptor_set;
    write.dstBinding = (uint32_t)index;
    write.descriptorCount = 1;
    write.descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    write.pBufferInfo = &buffer_info;
    vkUpdateDescriptorSets(context->device, 1, &write, 0, NULL);
    return 1;
}

int vk_compute_upload(vk_compute_context *context, int index,
    const void *data, size_t size) {
    if (context == NULL || index < 0 || index >= context->buffer_count) {
        return 0;
    }
    if (size > context->buffers[index].size || data == NULL) {
        return 0;
    }
    void *mapped = NULL;
    if (vkMapMemory(context->device, context->buffers[index].memory, 0,
            context->buffers[index].size, 0, &mapped) != VK_SUCCESS) {
        return 0;
    }
    memcpy(mapped, data, size);
    vkUnmapMemory(context->device, context->buffers[index].memory);
    return 1;
}

int vk_compute_upload_range(vk_compute_context *context, int index,
    size_t offset, const void *data, size_t size) {
    if (context == NULL || index < 0 || index >= context->buffer_count) {
        return 0;
    }
    if (data == NULL || offset > context->buffers[index].size ||
        size > context->buffers[index].size - offset) {
        return 0;
    }
    void *mapped = NULL;
    if (vkMapMemory(context->device, context->buffers[index].memory, 0,
            context->buffers[index].size, 0, &mapped) != VK_SUCCESS) {
        return 0;
    }
    memcpy((char *)mapped + offset, data, size);
    vkUnmapMemory(context->device, context->buffers[index].memory);
    return 1;
}

int vk_compute_readback(vk_compute_context *context, int index,
    void *data, size_t size) {
    if (context == NULL || index < 0 || index >= context->buffer_count) {
        return 0;
    }
    if (size > context->buffers[index].size || data == NULL) {
        return 0;
    }
    void *mapped = NULL;
    if (vkMapMemory(context->device, context->buffers[index].memory, 0,
            context->buffers[index].size, 0, &mapped) != VK_SUCCESS) {
        return 0;
    }
    memcpy(data, mapped, size);
    vkUnmapMemory(context->device, context->buffers[index].memory);
    return 1;
}

vk_compute_pipeline *vk_compute_pipeline_create(vk_compute_context *context,
    const uint32_t *code, size_t size, char *error, size_t error_size) {
    if (context == NULL || code == NULL || size == 0) {
        set_error(error, error_size, "invalid shader module");
        return NULL;
    }
    VkShaderModuleCreateInfo module_info = {0};
    module_info.sType = VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO;
    module_info.codeSize = size;
    module_info.pCode = code;
    VkShaderModule module = VK_NULL_HANDLE;
    if (vkCreateShaderModule(context->device, &module_info, NULL, &module) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create shader module");
        return NULL;
    }
    VkComputePipelineCreateInfo pipeline_info = {0};
    pipeline_info.sType = VK_STRUCTURE_TYPE_COMPUTE_PIPELINE_CREATE_INFO;
    pipeline_info.stage.sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    pipeline_info.stage.stage = VK_SHADER_STAGE_COMPUTE_BIT;
    pipeline_info.stage.module = module;
    pipeline_info.stage.pName = "main";
    pipeline_info.layout = context->pipeline_layout;
    vk_compute_pipeline *pipeline = calloc(1, sizeof(vk_compute_pipeline));
    if (pipeline == NULL ||
        vkCreateComputePipelines(context->device, VK_NULL_HANDLE, 1,
            &pipeline_info, NULL, &pipeline->pipeline) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create compute pipeline");
        vkDestroyShaderModule(context->device, module, NULL);
        free(pipeline);
        return NULL;
    }
    pipeline->module = module;
    return pipeline;
}

int vk_compute_begin(vk_compute_context *context, char *error, size_t error_size) {
    if (context == NULL) {
        set_error(error, error_size, "no context");
        return 0;
    }
    vkResetCommandBuffer(context->command, 0);
    VkCommandBufferBeginInfo begin = {0};
    begin.sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO;
    begin.flags = VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT;
    if (vkBeginCommandBuffer(context->command, &begin) != VK_SUCCESS) {
        set_error(error, error_size, "failed to begin command buffer");
        return 0;
    }
    VkMemoryBarrier before = {0};
    before.sType = VK_STRUCTURE_TYPE_MEMORY_BARRIER;
    before.srcAccessMask = VK_ACCESS_HOST_WRITE_BIT | VK_ACCESS_SHADER_WRITE_BIT;
    before.dstAccessMask = VK_ACCESS_SHADER_READ_BIT | VK_ACCESS_SHADER_WRITE_BIT;
    vkCmdPipelineBarrier(context->command,
        VK_PIPELINE_STAGE_HOST_BIT | VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT,
        VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT, 0, 1, &before, 0, NULL, 0, NULL);
    context->recording = 1;
    return 1;
}

int vk_compute_record(vk_compute_context *context, vk_compute_pipeline *pipeline,
    uint32_t groups_x, uint32_t groups_y, const void *push, size_t push_size,
    char *error, size_t error_size) {
    if (context == NULL || pipeline == NULL || groups_x == 0 || groups_y == 0) {
        set_error(error, error_size, "invalid dispatch");
        return 0;
    }
    if (!context->recording) {
        set_error(error, error_size, "compute batch is not recording");
        return 0;
    }
    vkCmdBindPipeline(context->command, VK_PIPELINE_BIND_POINT_COMPUTE,
        pipeline->pipeline);
    vkCmdBindDescriptorSets(context->command, VK_PIPELINE_BIND_POINT_COMPUTE,
        context->pipeline_layout, 0, 1, &context->descriptor_set, 0, NULL);
    if (push != NULL && push_size > 0) {
        vkCmdPushConstants(context->command, context->pipeline_layout,
            VK_SHADER_STAGE_COMPUTE_BIT, 0, (uint32_t)push_size, push);
    }
    vkCmdDispatch(context->command, groups_x, groups_y, 1);

    VkMemoryBarrier between = {0};
    between.sType = VK_STRUCTURE_TYPE_MEMORY_BARRIER;
    between.srcAccessMask = VK_ACCESS_SHADER_WRITE_BIT;
    between.dstAccessMask = VK_ACCESS_SHADER_READ_BIT | VK_ACCESS_SHADER_WRITE_BIT;
    vkCmdPipelineBarrier(context->command,
        VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT,
        VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT, 0, 1, &between, 0, NULL, 0, NULL);
    return 1;
}

int vk_compute_end(vk_compute_context *context, char *error, size_t error_size) {
    if (context == NULL || !context->recording) {
        set_error(error, error_size, "compute batch is not recording");
        return 0;
    }
    VkMemoryBarrier after = {0};
    after.sType = VK_STRUCTURE_TYPE_MEMORY_BARRIER;
    after.srcAccessMask = VK_ACCESS_SHADER_WRITE_BIT;
    after.dstAccessMask = VK_ACCESS_HOST_READ_BIT;
    vkCmdPipelineBarrier(context->command,
        VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT, VK_PIPELINE_STAGE_HOST_BIT,
        0, 1, &after, 0, NULL, 0, NULL);
    if (vkEndCommandBuffer(context->command) != VK_SUCCESS) {
        set_error(error, error_size, "failed to record compute batch");
        context->recording = 0;
        return 0;
    }
    context->recording = 0;
    VkSubmitInfo submit = {0};
    submit.sType = VK_STRUCTURE_TYPE_SUBMIT_INFO;
    submit.commandBufferCount = 1;
    submit.pCommandBuffers = &context->command;
    vkResetFences(context->device, 1, &context->fence);
    if (vkQueueSubmit(context->queue, 1, &submit, context->fence) != VK_SUCCESS ||
        vkWaitForFences(context->device, 1, &context->fence, VK_TRUE,
            UINT64_MAX) != VK_SUCCESS) {
        set_error(error, error_size, "compute batch failed");
        return 0;
    }
    return 1;
}

int vk_compute_dispatch(vk_compute_context *context, vk_compute_pipeline *pipeline,
    uint32_t groups_x, uint32_t groups_y, const void *push, size_t push_size,
    char *error, size_t error_size) {
    if (!vk_compute_begin(context, error, error_size)) {
        return 0;
    }
    if (!vk_compute_record(context, pipeline, groups_x, groups_y, push, push_size,
            error, error_size)) {
        context->recording = 0;
        return 0;
    }
    return vk_compute_end(context, error, error_size);
}

void vk_compute_pipeline_destroy(vk_compute_context *context,
    vk_compute_pipeline *pipeline) {
    if (context == NULL || pipeline == NULL) {
        return;
    }
    if (pipeline->pipeline != VK_NULL_HANDLE) {
        vkDestroyPipeline(context->device, pipeline->pipeline, NULL);
    }
    if (pipeline->module != VK_NULL_HANDLE) {
        vkDestroyShaderModule(context->device, pipeline->module, NULL);
    }
    free(pipeline);
}

uint64_t vk_compute_buffer_handle(vk_compute_context *context, int index) {
    if (context == NULL || index < 0 || index >= context->buffer_count) {
        return 0;
    }
    return (uint64_t)(uintptr_t)context->buffers[index].buffer;
}

void vk_compute_destroy(vk_compute_context *context) {
    if (context == NULL) {
        return;
    }
    if (context->device != VK_NULL_HANDLE) {
        vkDeviceWaitIdle(context->device);
        for (int index = 0; index < context->buffer_count; index++) {
            if (context->buffers[index].buffer != VK_NULL_HANDLE) {
                vkDestroyBuffer(context->device, context->buffers[index].buffer, NULL);
                vkFreeMemory(context->device, context->buffers[index].memory, NULL);
            }
        }
        if (context->descriptor_pool != VK_NULL_HANDLE) {
            vkDestroyDescriptorPool(context->device, context->descriptor_pool, NULL);
        }
        if (context->pipeline_layout != VK_NULL_HANDLE) {
            vkDestroyPipelineLayout(context->device, context->pipeline_layout, NULL);
        }
        if (context->set_layout != VK_NULL_HANDLE) {
            vkDestroyDescriptorSetLayout(context->device, context->set_layout, NULL);
        }
        if (context->fence != VK_NULL_HANDLE) {
            vkDestroyFence(context->device, context->fence, NULL);
        }
        if (context->command_pool != VK_NULL_HANDLE) {
            vkDestroyCommandPool(context->device, context->command_pool, NULL);
        }
        if (context->owns_device) {
            vkDestroyDevice(context->device, NULL);
        }
    }
    if (context->owns_device && context->instance != VK_NULL_HANDLE) {
        vkDestroyInstance(context->instance, NULL);
    }
    free(context);
}
