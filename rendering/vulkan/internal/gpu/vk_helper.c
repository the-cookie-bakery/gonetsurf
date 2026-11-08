#include <vulkan/vulkan.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

typedef struct {
    float rect[4];
    float uv[4];
    float color[4];
    float clip[4];
    float opacity;
    int mode;
    uint64_t texture;
} vk_draw;

typedef struct {
    VkImage image;
    VkDeviceMemory memory;
    VkImageView view;
    VkDescriptorSet descriptor;
    int width;
    int height;
    int format;
} vk_texture;

typedef struct vk_context {
    VkInstance instance;
    VkPhysicalDevice physical_device;
    VkDevice device;
    VkQueue queue;
    uint32_t queue_family;
    VkCommandPool command_pool;
    VkCommandBuffer command_buffer;
    VkFence fence;

    VkImage image;
    VkDeviceMemory image_memory;
    VkImageView image_view;
    VkRenderPass render_pass;
    VkFramebuffer framebuffer;
    VkDescriptorSetLayout descriptor_layout;
    VkPipelineLayout pipeline_layout;
    VkPipeline solid_pipeline;
    VkPipeline texture_pipeline;
    VkDescriptorPool descriptor_pool;
    VkSampler sampler;

    VkBuffer staging_buffer;
    VkDeviceMemory staging_memory;
    void *staging_pointer;

    vk_texture *textures;
    int texture_count;
    int texture_capacity;

    int width;
    int height;
} vk_context;

static void set_error(char *error, size_t error_size, const char *message) {
    if (error == NULL || error_size == 0) {
        return;
    }
    snprintf(error, error_size, "%s", message);
}

static uint32_t find_memory_type(vk_context *context, uint32_t type_bits, VkMemoryPropertyFlags properties) {
    VkPhysicalDeviceMemoryProperties memory_properties;
    vkGetPhysicalDeviceMemoryProperties(context->physical_device, &memory_properties);
    for (uint32_t index = 0; index < memory_properties.memoryTypeCount; index++) {
        if ((type_bits & (1u << index)) && (memory_properties.memoryTypes[index].propertyFlags & properties) == properties) {
            return index;
        }
    }
    return UINT32_MAX;
}

static VkShaderModule create_shader_module(vk_context *context, const uint32_t *code, size_t size) {
    VkShaderModuleCreateInfo info = {0};
    info.sType = VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO;
    info.codeSize = size;
    info.pCode = code;
    VkShaderModule module = VK_NULL_HANDLE;
    if (vkCreateShaderModule(context->device, &info, NULL, &module) != VK_SUCCESS) {
        return VK_NULL_HANDLE;
    }
    return module;
}

static int create_image(vk_context *context, uint32_t width, uint32_t height, VkFormat format, VkImageUsageFlags usage, VkImage *image, VkDeviceMemory *memory) {
    VkImageCreateInfo info = {0};
    info.sType = VK_STRUCTURE_TYPE_IMAGE_CREATE_INFO;
    info.imageType = VK_IMAGE_TYPE_2D;
    info.extent.width = width;
    info.extent.height = height;
    info.extent.depth = 1;
    info.mipLevels = 1;
    info.arrayLayers = 1;
    info.format = format;
    info.tiling = VK_IMAGE_TILING_OPTIMAL;
    info.initialLayout = VK_IMAGE_LAYOUT_UNDEFINED;
    info.usage = usage;
    info.samples = VK_SAMPLE_COUNT_1_BIT;
    info.sharingMode = VK_SHARING_MODE_EXCLUSIVE;
    if (vkCreateImage(context->device, &info, NULL, image) != VK_SUCCESS) {
        return 0;
    }
    VkMemoryRequirements requirements;
    vkGetImageMemoryRequirements(context->device, *image, &requirements);
    VkMemoryAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO;
    allocate.allocationSize = requirements.size;
    allocate.memoryTypeIndex = find_memory_type(context, requirements.memoryTypeBits, VK_MEMORY_PROPERTY_DEVICE_LOCAL_BIT);
    if (allocate.memoryTypeIndex == UINT32_MAX) {
        vkDestroyImage(context->device, *image, NULL);
        return 0;
    }
    if (vkAllocateMemory(context->device, &allocate, NULL, memory) != VK_SUCCESS) {
        vkDestroyImage(context->device, *image, NULL);
        return 0;
    }
    if (vkBindImageMemory(context->device, *image, *memory, 0) != VK_SUCCESS) {
        vkFreeMemory(context->device, *memory, NULL);
        vkDestroyImage(context->device, *image, NULL);
        return 0;
    }
    return 1;
}

static int create_buffer(vk_context *context, VkDeviceSize size, VkBufferUsageFlags usage, VkMemoryPropertyFlags properties, VkBuffer *buffer, VkDeviceMemory *memory) {
    VkBufferCreateInfo info = {0};
    info.sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO;
    info.size = size;
    info.usage = usage;
    info.sharingMode = VK_SHARING_MODE_EXCLUSIVE;
    if (vkCreateBuffer(context->device, &info, NULL, buffer) != VK_SUCCESS) {
        return 0;
    }
    VkMemoryRequirements requirements;
    vkGetBufferMemoryRequirements(context->device, *buffer, &requirements);
    VkMemoryAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO;
    allocate.allocationSize = requirements.size;
    allocate.memoryTypeIndex = find_memory_type(context, requirements.memoryTypeBits, properties);
    if (allocate.memoryTypeIndex == UINT32_MAX) {
        vkDestroyBuffer(context->device, *buffer, NULL);
        return 0;
    }
    if (vkAllocateMemory(context->device, &allocate, NULL, memory) != VK_SUCCESS) {
        vkDestroyBuffer(context->device, *buffer, NULL);
        return 0;
    }
    if (vkBindBufferMemory(context->device, *buffer, *memory, 0) != VK_SUCCESS) {
        vkFreeMemory(context->device, *memory, NULL);
        vkDestroyBuffer(context->device, *buffer, NULL);
        return 0;
    }
    return 1;
}

static VkCommandBuffer begin_once(vk_context *context) {
    VkCommandBufferBeginInfo begin = {0};
    begin.sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO;
    begin.flags = VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT;
    vkBeginCommandBuffer(context->command_buffer, &begin);
    return context->command_buffer;
}

static int end_once(vk_context *context) {
    vkEndCommandBuffer(context->command_buffer);
    VkSubmitInfo submit = {0};
    submit.sType = VK_STRUCTURE_TYPE_SUBMIT_INFO;
    submit.commandBufferCount = 1;
    submit.pCommandBuffers = &context->command_buffer;
    vkResetFences(context->device, 1, &context->fence);
    if (vkQueueSubmit(context->queue, 1, &submit, context->fence) != VK_SUCCESS) {
        return 0;
    }
    if (vkWaitForFences(context->device, 1, &context->fence, VK_TRUE, UINT64_MAX) != VK_SUCCESS) {
        return 0;
    }
    return 1;
}

static void image_barrier(VkCommandBuffer command, VkImage image, VkImageLayout from, VkImageLayout to, VkAccessFlags source_access, VkAccessFlags destination_access, VkPipelineStageFlags source_stage, VkPipelineStageFlags destination_stage) {
    VkImageMemoryBarrier barrier = {0};
    barrier.sType = VK_STRUCTURE_TYPE_IMAGE_MEMORY_BARRIER;
    barrier.oldLayout = from;
    barrier.newLayout = to;
    barrier.srcQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED;
    barrier.dstQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED;
    barrier.image = image;
    barrier.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    barrier.subresourceRange.levelCount = 1;
    barrier.subresourceRange.layerCount = 1;
    barrier.srcAccessMask = source_access;
    barrier.dstAccessMask = destination_access;
    vkCmdPipelineBarrier(command, source_stage, destination_stage, 0, 0, NULL, 0, NULL, 1, &barrier);
}

static int create_texture(vk_context *context, const uint8_t *pixels, int width, int height, int format, char *error, size_t error_size) {
    if (width <= 0 || height <= 0 || pixels == NULL) {
        set_error(error, error_size, "invalid texture dimensions");
        return 0;
    }
    if (context->texture_count >= context->texture_capacity) {
        int capacity = context->texture_capacity == 0 ? 64 : context->texture_capacity * 2;
        vk_texture *grown = realloc(context->textures, sizeof(vk_texture) * capacity);
        if (grown == NULL) {
            set_error(error, error_size, "out of memory for textures");
            return 0;
        }
        context->textures = grown;
        context->texture_capacity = capacity;
    }
    vk_texture texture = {0};
    texture.width = width;
    texture.height = height;
    texture.format = format;
    VkFormat vk_format = format == 1 ? VK_FORMAT_R8G8B8A8_UNORM : VK_FORMAT_R8_UNORM;
    if (!create_image(context, (uint32_t)width, (uint32_t)height, vk_format, VK_IMAGE_USAGE_SAMPLED_BIT | VK_IMAGE_USAGE_TRANSFER_DST_BIT, &texture.image, &texture.memory)) {
        set_error(error, error_size, "failed to create texture image");
        return 0;
    }
    VkDeviceSize byte_size = (VkDeviceSize)width * height * (format == 1 ? 4 : 1);
    VkBuffer staging = VK_NULL_HANDLE;
    VkDeviceMemory staging_memory = VK_NULL_HANDLE;
    if (!create_buffer(context, byte_size, VK_BUFFER_USAGE_TRANSFER_SRC_BIT, VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT, &staging, &staging_memory)) {
        set_error(error, error_size, "failed to create texture staging buffer");
        vkDestroyImage(context->device, texture.image, NULL);
        vkFreeMemory(context->device, texture.memory, NULL);
        return 0;
    }
    void *mapped = NULL;
    vkMapMemory(context->device, staging_memory, 0, byte_size, 0, &mapped);
    memcpy(mapped, pixels, byte_size);
    vkUnmapMemory(context->device, staging_memory);

    VkCommandBuffer command = begin_once(context);
    image_barrier(command, texture.image, VK_IMAGE_LAYOUT_UNDEFINED, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 0, VK_ACCESS_TRANSFER_WRITE_BIT, VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT, VK_PIPELINE_STAGE_TRANSFER_BIT);
    VkBufferImageCopy region = {0};
    region.imageSubresource.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    region.imageSubresource.layerCount = 1;
    region.imageExtent.width = (uint32_t)width;
    region.imageExtent.height = (uint32_t)height;
    region.imageExtent.depth = 1;
    vkCmdCopyBufferToImage(command, staging, texture.image, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 1, &region);
    image_barrier(command, texture.image, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL, VK_ACCESS_TRANSFER_WRITE_BIT, VK_ACCESS_SHADER_READ_BIT, VK_PIPELINE_STAGE_TRANSFER_BIT, VK_PIPELINE_STAGE_FRAGMENT_SHADER_BIT);
    if (!end_once(context)) {
        set_error(error, error_size, "failed to upload texture");
        vkDestroyBuffer(context->device, staging, NULL);
        vkFreeMemory(context->device, staging_memory, NULL);
        vkDestroyImage(context->device, texture.image, NULL);
        vkFreeMemory(context->device, texture.memory, NULL);
        return 0;
    }
    vkDestroyBuffer(context->device, staging, NULL);
    vkFreeMemory(context->device, staging_memory, NULL);

    VkImageViewCreateInfo view_info = {0};
    view_info.sType = VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO;
    view_info.image = texture.image;
    view_info.viewType = VK_IMAGE_VIEW_TYPE_2D;
    view_info.format = vk_format;
    view_info.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    view_info.subresourceRange.levelCount = 1;
    view_info.subresourceRange.layerCount = 1;
    if (vkCreateImageView(context->device, &view_info, NULL, &texture.view) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create texture view");
        return 0;
    }

    VkDescriptorSetAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
    allocate.descriptorPool = context->descriptor_pool;
    allocate.descriptorSetCount = 1;
    allocate.pSetLayouts = &context->descriptor_layout;
    if (vkAllocateDescriptorSets(context->device, &allocate, &texture.descriptor) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate texture descriptor");
        return 0;
    }
    VkDescriptorImageInfo image_info = {0};
    image_info.imageLayout = VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL;
    image_info.imageView = texture.view;
    image_info.sampler = context->sampler;
    VkWriteDescriptorSet write = {0};
    write.sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
    write.dstSet = texture.descriptor;
    write.dstBinding = 0;
    write.descriptorCount = 1;
    write.descriptorType = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
    write.pImageInfo = &image_info;
    vkUpdateDescriptorSets(context->device, 1, &write, 0, NULL);

    context->textures[context->texture_count] = texture;
    context->texture_count++;
    return context->texture_count;
}

static int create_pipeline(vk_context *context, VkShaderModule vertex, VkShaderModule fragment, int blending, VkPipeline *pipeline) {
    VkPipelineShaderStageCreateInfo stages[2] = {0};
    stages[0].sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    stages[0].stage = VK_SHADER_STAGE_VERTEX_BIT;
    stages[0].module = vertex;
    stages[0].pName = "main";
    stages[1].sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    stages[1].stage = VK_SHADER_STAGE_FRAGMENT_BIT;
    stages[1].module = fragment;
    stages[1].pName = "main";

    VkPipelineVertexInputStateCreateInfo vertex_input = {0};
    vertex_input.sType = VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO;
    VkPipelineInputAssemblyStateCreateInfo assembly = {0};
    assembly.sType = VK_STRUCTURE_TYPE_PIPELINE_INPUT_ASSEMBLY_STATE_CREATE_INFO;
    assembly.topology = VK_PRIMITIVE_TOPOLOGY_TRIANGLE_STRIP;
    VkViewport viewport = {0, 0, (float)context->width, (float)context->height, 0, 1};
    VkRect2D scissor = {{0, 0}, {(uint32_t)context->width, (uint32_t)context->height}};
    VkPipelineViewportStateCreateInfo viewport_state = {0};
    viewport_state.sType = VK_STRUCTURE_TYPE_PIPELINE_VIEWPORT_STATE_CREATE_INFO;
    viewport_state.viewportCount = 1;
    viewport_state.pViewports = &viewport;
    viewport_state.scissorCount = 1;
    viewport_state.pScissors = &scissor;
    VkPipelineRasterizationStateCreateInfo raster = {0};
    raster.sType = VK_STRUCTURE_TYPE_PIPELINE_RASTERIZATION_STATE_CREATE_INFO;
    raster.polygonMode = VK_POLYGON_MODE_FILL;
    raster.cullMode = VK_CULL_MODE_NONE;
    raster.frontFace = VK_FRONT_FACE_COUNTER_CLOCKWISE;
    raster.lineWidth = 1;
    VkPipelineMultisampleStateCreateInfo multisample = {0};
    multisample.sType = VK_STRUCTURE_TYPE_PIPELINE_MULTISAMPLE_STATE_CREATE_INFO;
    multisample.rasterizationSamples = VK_SAMPLE_COUNT_1_BIT;

    VkPipelineColorBlendAttachmentState attachment = {0};
    attachment.blendEnable = blending ? VK_TRUE : VK_FALSE;
    attachment.srcColorBlendFactor = VK_BLEND_FACTOR_ONE;
    attachment.dstColorBlendFactor = VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA;
    attachment.colorBlendOp = VK_BLEND_OP_ADD;
    attachment.srcAlphaBlendFactor = VK_BLEND_FACTOR_ONE;
    attachment.dstAlphaBlendFactor = VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA;
    attachment.alphaBlendOp = VK_BLEND_OP_ADD;
    attachment.colorWriteMask = VK_COLOR_COMPONENT_R_BIT | VK_COLOR_COMPONENT_G_BIT | VK_COLOR_COMPONENT_B_BIT | VK_COLOR_COMPONENT_A_BIT;
    VkDynamicState dynamic_states[2] = {VK_DYNAMIC_STATE_VIEWPORT, VK_DYNAMIC_STATE_SCISSOR};
    VkPipelineDynamicStateCreateInfo dynamic_state = {0};
    dynamic_state.sType = VK_STRUCTURE_TYPE_PIPELINE_DYNAMIC_STATE_CREATE_INFO;
    dynamic_state.dynamicStateCount = 2;
    dynamic_state.pDynamicStates = dynamic_states;
    VkPipelineColorBlendStateCreateInfo blend = {0};
    blend.sType = VK_STRUCTURE_TYPE_PIPELINE_COLOR_BLEND_STATE_CREATE_INFO;
    blend.attachmentCount = 1;
    blend.pAttachments = &attachment;

    VkGraphicsPipelineCreateInfo info = {0};
    info.sType = VK_STRUCTURE_TYPE_GRAPHICS_PIPELINE_CREATE_INFO;
    info.stageCount = 2;
    info.pStages = stages;
    info.pVertexInputState = &vertex_input;
    info.pInputAssemblyState = &assembly;
    info.pViewportState = &viewport_state;
    info.pRasterizationState = &raster;
    info.pMultisampleState = &multisample;
    info.pColorBlendState = &blend;
    info.pDynamicState = &dynamic_state;
    info.layout = context->pipeline_layout;
    info.renderPass = context->render_pass;
    info.subpass = 0;
    return vkCreateGraphicsPipelines(context->device, VK_NULL_HANDLE, 1, &info, NULL, pipeline) == VK_SUCCESS;
}

vk_context *vk_context_create(int width, int height,
    const uint32_t *vertex_code, size_t vertex_size,
    const uint32_t *solid_code, size_t solid_size,
    const uint32_t *texture_code, size_t texture_size,
    char *error, size_t error_size) {
    vk_context *context = calloc(1, sizeof(vk_context));
    if (context == NULL) {
        set_error(error, error_size, "out of memory");
        return NULL;
    }
    context->width = width;
    context->height = height;

    VkApplicationInfo application = {0};
    application.sType = VK_STRUCTURE_TYPE_APPLICATION_INFO;
    application.pApplicationName = "gonetsurf";
    application.apiVersion = VK_API_VERSION_1_0;
    VkInstanceCreateInfo instance_info = {0};
    instance_info.sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO;
    instance_info.pApplicationInfo = &application;
    if (vkCreateInstance(&instance_info, NULL, &context->instance) != VK_SUCCESS) {
        set_error(error, error_size, "vkCreateInstance failed");
        return NULL;
    }

    uint32_t device_count = 0;
    vkEnumeratePhysicalDevices(context->instance, &device_count, NULL);
    if (device_count == 0) {
        set_error(error, error_size, "no Vulkan physical devices");
        return NULL;
    }
    VkPhysicalDevice *devices = malloc(sizeof(VkPhysicalDevice) * device_count);
    vkEnumeratePhysicalDevices(context->instance, &device_count, devices);
    context->physical_device = devices[0];
    for (uint32_t index = 0; index < device_count; index++) {
        VkPhysicalDeviceProperties properties;
        vkGetPhysicalDeviceProperties(devices[index], &properties);
        if (properties.deviceType == VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU) {
            context->physical_device = devices[index];
            break;
        }
    }
    free(devices);

    uint32_t family_count = 0;
    vkGetPhysicalDeviceQueueFamilyProperties(context->physical_device, &family_count, NULL);
    VkQueueFamilyProperties *families = malloc(sizeof(VkQueueFamilyProperties) * family_count);
    vkGetPhysicalDeviceQueueFamilyProperties(context->physical_device, &family_count, families);
    context->queue_family = UINT32_MAX;
    for (uint32_t index = 0; index < family_count; index++) {
        if (families[index].queueFlags & VK_QUEUE_GRAPHICS_BIT) {
            context->queue_family = index;
            break;
        }
    }
    free(families);
    if (context->queue_family == UINT32_MAX) {
        set_error(error, error_size, "no graphics queue family");
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
    if (vkCreateDevice(context->physical_device, &device_info, NULL, &context->device) != VK_SUCCESS) {
        set_error(error, error_size, "vkCreateDevice failed");
        return NULL;
    }
    vkGetDeviceQueue(context->device, context->queue_family, 0, &context->queue);

    VkCommandPoolCreateInfo pool_info = {0};
    pool_info.sType = VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO;
    pool_info.flags = VK_COMMAND_POOL_CREATE_RESET_COMMAND_BUFFER_BIT;
    pool_info.queueFamilyIndex = context->queue_family;
    vkCreateCommandPool(context->device, &pool_info, NULL, &context->command_pool);
    VkCommandBufferAllocateInfo command_allocate = {0};
    command_allocate.sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO;
    command_allocate.commandPool = context->command_pool;
    command_allocate.level = VK_COMMAND_BUFFER_LEVEL_PRIMARY;
    command_allocate.commandBufferCount = 1;
    vkAllocateCommandBuffers(context->device, &command_allocate, &context->command_buffer);
    VkFenceCreateInfo fence_info = {0};
    fence_info.sType = VK_STRUCTURE_TYPE_FENCE_CREATE_INFO;
    vkCreateFence(context->device, &fence_info, NULL, &context->fence);

    if (!create_image(context, (uint32_t)width, (uint32_t)height, VK_FORMAT_R8G8B8A8_UNORM, VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT | VK_IMAGE_USAGE_TRANSFER_SRC_BIT, &context->image, &context->image_memory)) {
        set_error(error, error_size, "failed to create offscreen image");
        return NULL;
    }
    VkImageViewCreateInfo view_info = {0};
    view_info.sType = VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO;
    view_info.image = context->image;
    view_info.viewType = VK_IMAGE_VIEW_TYPE_2D;
    view_info.format = VK_FORMAT_R8G8B8A8_UNORM;
    view_info.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    view_info.subresourceRange.levelCount = 1;
    view_info.subresourceRange.layerCount = 1;
    vkCreateImageView(context->device, &view_info, NULL, &context->image_view);

    VkAttachmentDescription attachment = {0};
    attachment.format = VK_FORMAT_R8G8B8A8_UNORM;
    attachment.samples = VK_SAMPLE_COUNT_1_BIT;
    attachment.loadOp = VK_ATTACHMENT_LOAD_OP_CLEAR;
    attachment.storeOp = VK_ATTACHMENT_STORE_OP_STORE;
    attachment.stencilLoadOp = VK_ATTACHMENT_LOAD_OP_DONT_CARE;
    attachment.stencilStoreOp = VK_ATTACHMENT_STORE_OP_DONT_CARE;
    attachment.initialLayout = VK_IMAGE_LAYOUT_UNDEFINED;
    attachment.finalLayout = VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL;
    VkAttachmentReference reference = {0, VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL};
    VkSubpassDescription subpass = {0};
    subpass.pipelineBindPoint = VK_PIPELINE_BIND_POINT_GRAPHICS;
    subpass.colorAttachmentCount = 1;
    subpass.pColorAttachments = &reference;
    VkRenderPassCreateInfo render_info = {0};
    render_info.sType = VK_STRUCTURE_TYPE_RENDER_PASS_CREATE_INFO;
    render_info.attachmentCount = 1;
    render_info.pAttachments = &attachment;
    render_info.subpassCount = 1;
    render_info.pSubpasses = &subpass;
    if (vkCreateRenderPass(context->device, &render_info, NULL, &context->render_pass) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create render pass");
        return NULL;
    }
    VkImageView attachments[1] = {context->image_view};
    VkFramebufferCreateInfo framebuffer_info = {0};
    framebuffer_info.sType = VK_STRUCTURE_TYPE_FRAMEBUFFER_CREATE_INFO;
    framebuffer_info.renderPass = context->render_pass;
    framebuffer_info.attachmentCount = 1;
    framebuffer_info.pAttachments = attachments;
    framebuffer_info.width = (uint32_t)width;
    framebuffer_info.height = (uint32_t)height;
    framebuffer_info.layers = 1;
    vkCreateFramebuffer(context->device, &framebuffer_info, NULL, &context->framebuffer);

    VkDescriptorSetLayoutBinding binding = {0};
    binding.binding = 0;
    binding.descriptorType = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
    binding.descriptorCount = 1;
    binding.stageFlags = VK_SHADER_STAGE_FRAGMENT_BIT;
    VkDescriptorSetLayoutCreateInfo layout_info = {0};
    layout_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO;
    layout_info.bindingCount = 1;
    layout_info.pBindings = &binding;
    vkCreateDescriptorSetLayout(context->device, &layout_info, NULL, &context->descriptor_layout);
    VkPushConstantRange range = {0};
    range.stageFlags = VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT;
    range.offset = 0;
    range.size = sizeof(float) * 16;
    VkPipelineLayoutCreateInfo pipeline_layout_info = {0};
    pipeline_layout_info.sType = VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO;
    pipeline_layout_info.setLayoutCount = 1;
    pipeline_layout_info.pSetLayouts = &context->descriptor_layout;
    pipeline_layout_info.pushConstantRangeCount = 1;
    pipeline_layout_info.pPushConstantRanges = &range;
    vkCreatePipelineLayout(context->device, &pipeline_layout_info, NULL, &context->pipeline_layout);

    VkDescriptorPoolSize pool_size = {0};
    pool_size.type = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
    pool_size.descriptorCount = 4096;
    VkDescriptorPoolCreateInfo descriptor_pool_info = {0};
    descriptor_pool_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO;
    descriptor_pool_info.maxSets = 4096;
    descriptor_pool_info.poolSizeCount = 1;
    descriptor_pool_info.pPoolSizes = &pool_size;
    vkCreateDescriptorPool(context->device, &descriptor_pool_info, NULL, &context->descriptor_pool);

    VkSamplerCreateInfo sampler_info = {0};
    sampler_info.sType = VK_STRUCTURE_TYPE_SAMPLER_CREATE_INFO;
    sampler_info.magFilter = VK_FILTER_LINEAR;
    sampler_info.minFilter = VK_FILTER_LINEAR;
    sampler_info.addressModeU = VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE;
    sampler_info.addressModeV = VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE;
    sampler_info.addressModeW = VK_SAMPLER_ADDRESS_MODE_CLAMP_TO_EDGE;
    sampler_info.maxLod = 1;
    vkCreateSampler(context->device, &sampler_info, NULL, &context->sampler);

    VkShaderModule vertex = create_shader_module(context, vertex_code, vertex_size);
    VkShaderModule solid = create_shader_module(context, solid_code, solid_size);
    VkShaderModule texture = create_shader_module(context, texture_code, texture_size);
    if (vertex == VK_NULL_HANDLE || solid == VK_NULL_HANDLE || texture == VK_NULL_HANDLE) {
        set_error(error, error_size, "failed to create shader modules");
        return NULL;
    }
    if (!create_pipeline(context, vertex, solid, 0, &context->solid_pipeline) ||
        !create_pipeline(context, vertex, texture, 1, &context->texture_pipeline)) {
        set_error(error, error_size, "failed to create pipelines");
        return NULL;
    }
    vkDestroyShaderModule(context->device, vertex, NULL);
    vkDestroyShaderModule(context->device, solid, NULL);
    vkDestroyShaderModule(context->device, texture, NULL);

    VkDeviceSize staging_size = (VkDeviceSize)width * height * 4;
    create_buffer(context, staging_size, VK_BUFFER_USAGE_TRANSFER_DST_BIT, VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT, &context->staging_buffer, &context->staging_memory);
    vkMapMemory(context->device, context->staging_memory, 0, staging_size, 0, &context->staging_pointer);

    uint8_t white_pixel[4] = {255, 255, 255, 255};
    create_texture(context, white_pixel, 1, 1, 1, error, error_size);
    return context;
}

int vk_context_create_texture(vk_context *context, const uint8_t *pixels, int width, int height, int format, char *error, size_t error_size) {
    if (context == NULL) {
        set_error(error, error_size, "context is null");
        return 0;
    }
    return create_texture(context, pixels, width, height, format, error, error_size);
}

int vk_context_render(vk_context *context, const float clear[4], const vk_draw *draws, int draw_count, uint8_t *out_pixels, char *error, size_t error_size) {
    if (context == NULL) {
        set_error(error, error_size, "context is null");
        return 0;
    }
    VkCommandBuffer command = begin_once(context);
    VkClearValue clear_value = {0};
    clear_value.color.float32[0] = clear[0];
    clear_value.color.float32[1] = clear[1];
    clear_value.color.float32[2] = clear[2];
    clear_value.color.float32[3] = clear[3];
    VkRenderPassBeginInfo render_begin = {0};
    render_begin.sType = VK_STRUCTURE_TYPE_RENDER_PASS_BEGIN_INFO;
    render_begin.renderPass = context->render_pass;
    render_begin.framebuffer = context->framebuffer;
    render_begin.renderArea.extent.width = (uint32_t)context->width;
    render_begin.renderArea.extent.height = (uint32_t)context->height;
    render_begin.clearValueCount = 1;
    render_begin.pClearValues = &clear_value;
    vkCmdBeginRenderPass(command, &render_begin, VK_SUBPASS_CONTENTS_INLINE);
    VkViewport viewport = {0, 0, (float)context->width, (float)context->height, 0, 1};
    VkRect2D scissor = {{0, 0}, {(uint32_t)context->width, (uint32_t)context->height}};
    vkCmdSetViewport(command, 0, 1, &viewport);
    vkCmdSetScissor(command, 0, 1, &scissor);

    VkPipeline bound_pipeline = VK_NULL_HANDLE;
    for (int index = 0; index < draw_count; index++) {
        const vk_draw *draw = &draws[index];
        VkPipeline pipeline = draw->texture > 0 ? context->texture_pipeline : context->solid_pipeline;
        if (pipeline != bound_pipeline) {
            vkCmdBindPipeline(command, VK_PIPELINE_BIND_POINT_GRAPHICS, pipeline);
            bound_pipeline = pipeline;
        }
        int clip_x = (int)(draw->clip[0] < 0 ? 0 : draw->clip[0]);
        int clip_y = (int)(draw->clip[1] < 0 ? 0 : draw->clip[1]);
        int clip_width = (int)(draw->clip[2] > 0 ? draw->clip[2] : 0);
        int clip_height = (int)(draw->clip[3] > 0 ? draw->clip[3] : 0);
        if (clip_x + clip_width > context->width) clip_width = context->width - clip_x;
        if (clip_y + clip_height > context->height) clip_height = context->height - clip_y;
        if (clip_width <= 0 || clip_height <= 0) {
            continue;
        }
        VkRect2D draw_scissor = {{clip_x, clip_y}, {(uint32_t)clip_width, (uint32_t)clip_height}};
        vkCmdSetScissor(command, 0, 1, &draw_scissor);
        uint64_t texture_index = draw->texture > 0 ? draw->texture - 1 : 0;
        if (texture_index < (uint64_t)context->texture_count) {
            vkCmdBindDescriptorSets(command, VK_PIPELINE_BIND_POINT_GRAPHICS, context->pipeline_layout, 0, 1, &context->textures[texture_index].descriptor, 0, NULL);
        }
        float push[16];
        memcpy(&push[0], draw->rect, sizeof(float) * 4);
        memcpy(&push[4], draw->uv, sizeof(float) * 4);
        memcpy(&push[8], draw->color, sizeof(float) * 4);
        push[12] = draw->opacity;
        push[13] = (float)draw->mode;
        push[14] = (float)context->width;
        push[15] = (float)context->height;
        vkCmdPushConstants(command, context->pipeline_layout, VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT, 0, sizeof(push), push);
        vkCmdDraw(command, 4, 1, 0, 0);
    }
    vkCmdEndRenderPass(command);
    VkMemoryBarrier barrier = {0};
    barrier.sType = VK_STRUCTURE_TYPE_MEMORY_BARRIER;
    barrier.srcAccessMask = VK_ACCESS_COLOR_ATTACHMENT_WRITE_BIT;
    barrier.dstAccessMask = VK_ACCESS_TRANSFER_READ_BIT;
    vkCmdPipelineBarrier(command, VK_PIPELINE_STAGE_COLOR_ATTACHMENT_OUTPUT_BIT, VK_PIPELINE_STAGE_TRANSFER_BIT, 0, 1, &barrier, 0, NULL, 0, NULL);
    VkBufferImageCopy region = {0};
    region.imageSubresource.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    region.imageSubresource.layerCount = 1;
    region.imageExtent.width = (uint32_t)context->width;
    region.imageExtent.height = (uint32_t)context->height;
    region.imageExtent.depth = 1;
    vkCmdCopyImageToBuffer(command, context->image, VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL, context->staging_buffer, 1, &region);
    if (!end_once(context)) {
        set_error(error, error_size, "render submission failed");
        return 0;
    }
    memcpy(out_pixels, context->staging_pointer, (size_t)context->width * context->height * 4);
    return 1;
}

void vk_context_destroy(vk_context *context) {
    if (context == NULL) {
        return;
    }
    if (context->device != VK_NULL_HANDLE) {
        vkDeviceWaitIdle(context->device);
        if (context->staging_pointer != NULL) {
            vkUnmapMemory(context->device, context->staging_memory);
        }
        if (context->staging_buffer != VK_NULL_HANDLE) {
            vkDestroyBuffer(context->device, context->staging_buffer, NULL);
        }
        if (context->staging_memory != VK_NULL_HANDLE) {
            vkFreeMemory(context->device, context->staging_memory, NULL);
        }
        for (int index = 0; index < context->texture_count; index++) {
            vk_texture *texture = &context->textures[index];
            if (texture->view != VK_NULL_HANDLE) {
                vkDestroyImageView(context->device, texture->view, NULL);
            }
            if (texture->image != VK_NULL_HANDLE) {
                vkDestroyImage(context->device, texture->image, NULL);
            }
            if (texture->memory != VK_NULL_HANDLE) {
                vkFreeMemory(context->device, texture->memory, NULL);
            }
        }
        free(context->textures);
        if (context->solid_pipeline != VK_NULL_HANDLE) vkDestroyPipeline(context->device, context->solid_pipeline, NULL);
        if (context->texture_pipeline != VK_NULL_HANDLE) vkDestroyPipeline(context->device, context->texture_pipeline, NULL);
        if (context->pipeline_layout != VK_NULL_HANDLE) vkDestroyPipelineLayout(context->device, context->pipeline_layout, NULL);
        if (context->descriptor_pool != VK_NULL_HANDLE) vkDestroyDescriptorPool(context->device, context->descriptor_pool, NULL);
        if (context->descriptor_layout != VK_NULL_HANDLE) vkDestroyDescriptorSetLayout(context->device, context->descriptor_layout, NULL);
        if (context->sampler != VK_NULL_HANDLE) vkDestroySampler(context->device, context->sampler, NULL);
        if (context->framebuffer != VK_NULL_HANDLE) vkDestroyFramebuffer(context->device, context->framebuffer, NULL);
        if (context->render_pass != VK_NULL_HANDLE) vkDestroyRenderPass(context->device, context->render_pass, NULL);
        if (context->image_view != VK_NULL_HANDLE) vkDestroyImageView(context->device, context->image_view, NULL);
        if (context->image != VK_NULL_HANDLE) vkDestroyImage(context->device, context->image, NULL);
        if (context->image_memory != VK_NULL_HANDLE) vkFreeMemory(context->device, context->image_memory, NULL);
        if (context->fence != VK_NULL_HANDLE) vkDestroyFence(context->device, context->fence, NULL);
        if (context->command_pool != VK_NULL_HANDLE) vkDestroyCommandPool(context->device, context->command_pool, NULL);
        vkDestroyDevice(context->device, NULL);
    }
    if (context->instance != VK_NULL_HANDLE) {
        vkDestroyInstance(context->instance, NULL);
    }
    free(context);
}
