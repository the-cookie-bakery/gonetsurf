#define VK_USE_PLATFORM_XCB_KHR
#include <vulkan/vulkan.h>
#include <xcb/xcb.h>
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
    float transform[6];
    int transformed;
} vk_draw;

typedef struct {
    VkImage image;
    VkDeviceMemory memory;
    VkImageView view;
    VkDescriptorSet descriptor;
    int width;
    int height;
    int format;
    int owns_image;
} vk_texture;

typedef struct {
    VkImage image;
    VkDeviceMemory memory;
    VkImageView view;
    VkFramebuffer framebuffer;
    VkDescriptorSet descriptor;
    int texture_index;
    int width;
    int height;
} vk_layer_target;

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
    VkRenderPass layer_render_pass;
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

    VkDescriptorSetLayout path_set_layout;
    VkDescriptorPool path_descriptor_pool;
    VkDescriptorSet path_descriptor;
    VkPipelineLayout path_pipeline_layout;
    VkPipeline path_pipeline;
    VkBuffer edge_buffer;
    VkDeviceMemory edge_memory;
    VkDeviceSize edge_capacity;
    VkBuffer coverage_buffer;
    VkDeviceMemory coverage_memory;
    VkDeviceSize coverage_capacity;
    VkImage coverage_image;
    VkDeviceMemory coverage_image_memory;
    VkImageView coverage_image_view;
    int coverage_texture_index;
    int coverage_width;
    int coverage_height;

    xcb_connection_t *xcb_connection;
    VkSurfaceKHR surface;
    VkSwapchainKHR swapchain;
    VkFormat swapchain_format;
    VkExtent2D swapchain_extent;
    int swapchain_bgra;
    VkImage *swapchain_images;
    uint32_t swapchain_image_count;
    VkImageView *swapchain_views;
    VkSemaphore image_available;
    VkSemaphore render_finished;
    VkBuffer present_buffer;
    VkDeviceMemory present_memory;
    VkDeviceSize present_capacity;
    VkBuffer dummy_node_buffer;
    VkDeviceMemory dummy_node_memory;
    VkImage geometry_image;
    VkDeviceMemory geometry_image_memory;
    VkImageView geometry_image_view;
    VkDescriptorSet geometry_descriptor;
    int geometry_texture_index;
    PFN_vkGetPhysicalDeviceSurfaceCapabilitiesKHR pGetSurfaceCapabilities;
    PFN_vkGetPhysicalDeviceSurfaceFormatsKHR pGetSurfaceFormats;
    PFN_vkGetPhysicalDeviceSurfacePresentModesKHR pGetSurfacePresentModes;
    PFN_vkCreateSwapchainKHR pCreateSwapchain;
    PFN_vkDestroySwapchainKHR pDestroySwapchain;
    PFN_vkGetSwapchainImagesKHR pGetSwapchainImages;
    PFN_vkAcquireNextImageKHR pAcquireNextImage;
    PFN_vkQueuePresentKHR pQueuePresent;

    vk_texture *textures;
    int texture_count;
    int texture_capacity;
    vk_layer_target *layers;
    int layer_count;
    int layer_capacity;
    int layer_cursor;

    int width;
    int height;
    int owns_device;
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

static void write_texture_descriptor(vk_context *context, VkDescriptorSet set,
    VkImageView first, VkImageView second) {
    VkDescriptorImageInfo infos[2];
    memset(infos, 0, sizeof(infos));
    for (int index = 0; index < 2; index++) {
        infos[index].imageLayout = VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL;
        infos[index].imageView = index == 0 ? first : second;
        infos[index].sampler = context->sampler;
    }
    VkDescriptorBufferInfo buffer_info = {0};
    buffer_info.buffer = context->dummy_node_buffer;
    buffer_info.offset = 0;
    buffer_info.range = 16;
    VkWriteDescriptorSet writes[3];
    memset(writes, 0, sizeof(writes));
    for (int index = 0; index < 2; index++) {
        writes[index].sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
        writes[index].dstSet = set;
        writes[index].dstBinding = (uint32_t)index;
        writes[index].descriptorCount = 1;
        writes[index].descriptorType = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
        writes[index].pImageInfo = &infos[index];
    }
    writes[2].sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
    writes[2].dstSet = set;
    writes[2].dstBinding = 2;
    writes[2].descriptorCount = 1;
    writes[2].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    writes[2].pBufferInfo = &buffer_info;
    vkUpdateDescriptorSets(context->device, 3, writes, 0, NULL);
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
    texture.owns_image = 1;
    write_texture_descriptor(context, texture.descriptor, texture.view, texture.view);

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
    uintptr_t instance_handle, uintptr_t physical_handle, uintptr_t device_handle,
    uintptr_t queue_handle, uint32_t queue_family,
    char *error, size_t error_size) {
    vk_context *context = calloc(1, sizeof(vk_context));
    if (context == NULL) {
        set_error(error, error_size, "out of memory");
        return NULL;
    }
    context->width = width;
    context->height = height;

    if (device_handle != 0) {
        context->instance = (VkInstance)instance_handle;
        context->physical_device = (VkPhysicalDevice)physical_handle;
        context->device = (VkDevice)device_handle;
        context->queue = (VkQueue)queue_handle;
        context->queue_family = queue_family;
        context->owns_device = 0;
    } else {
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
    context->owns_device = 1;
    }

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
    VkAttachmentDescription layer_attachment = attachment;
    layer_attachment.finalLayout = VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL;
    VkRenderPassCreateInfo layer_render_info = render_info;
    layer_render_info.pAttachments = &layer_attachment;
    if (vkCreateRenderPass(context->device, &layer_render_info, NULL, &context->layer_render_pass) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create layer render pass");
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

    VkDescriptorSetLayoutBinding bindings[3] = {0};
    for (int binding_index = 0; binding_index < 2; binding_index++) {
        bindings[binding_index].binding = (uint32_t)binding_index;
        bindings[binding_index].descriptorType = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
        bindings[binding_index].descriptorCount = 1;
        bindings[binding_index].stageFlags = VK_SHADER_STAGE_FRAGMENT_BIT;
    }
    bindings[2].binding = 2;
    bindings[2].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    bindings[2].descriptorCount = 1;
    bindings[2].stageFlags = VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT;
    VkDescriptorSetLayoutCreateInfo layout_info = {0};
    layout_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO;
    layout_info.bindingCount = 3;
    layout_info.pBindings = bindings;
    vkCreateDescriptorSetLayout(context->device, &layout_info, NULL, &context->descriptor_layout);
    VkPushConstantRange range = {0};
    range.stageFlags = VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT;
    range.offset = 0;
    range.size = sizeof(float) * 24;
    VkPipelineLayoutCreateInfo pipeline_layout_info = {0};
    pipeline_layout_info.sType = VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO;
    pipeline_layout_info.setLayoutCount = 1;
    pipeline_layout_info.pSetLayouts = &context->descriptor_layout;
    pipeline_layout_info.pushConstantRangeCount = 1;
    pipeline_layout_info.pPushConstantRanges = &range;
    vkCreatePipelineLayout(context->device, &pipeline_layout_info, NULL, &context->pipeline_layout);

    VkDescriptorPoolSize pool_sizes[2] = {0};
    pool_sizes[0].type = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
    pool_sizes[0].descriptorCount = 8192;
    pool_sizes[1].type = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    pool_sizes[1].descriptorCount = 4096;
    VkDescriptorPoolCreateInfo descriptor_pool_info = {0};
    descriptor_pool_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO;
    descriptor_pool_info.maxSets = 4096;
    descriptor_pool_info.poolSizeCount = 2;
    descriptor_pool_info.pPoolSizes = pool_sizes;
    vkCreateDescriptorPool(context->device, &descriptor_pool_info, NULL, &context->descriptor_pool);
    VkBufferCreateInfo dummy_info = {0};
    dummy_info.sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO;
    dummy_info.size = 16;
    dummy_info.usage = VK_BUFFER_USAGE_STORAGE_BUFFER_BIT;
    dummy_info.sharingMode = VK_SHARING_MODE_EXCLUSIVE;
    vkCreateBuffer(context->device, &dummy_info, NULL, &context->dummy_node_buffer);
    VkMemoryRequirements dummy_requirements;
    vkGetBufferMemoryRequirements(context->device, context->dummy_node_buffer, &dummy_requirements);
    VkMemoryAllocateInfo dummy_allocate = {0};
    dummy_allocate.sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO;
    dummy_allocate.allocationSize = dummy_requirements.size;
    dummy_allocate.memoryTypeIndex = find_memory_type(context, dummy_requirements.memoryTypeBits, VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT);
    vkAllocateMemory(context->device, &dummy_allocate, NULL, &context->dummy_node_memory);
    vkBindBufferMemory(context->device, context->dummy_node_buffer, context->dummy_node_memory, 0);

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

static void record_draws(vk_context *context, VkCommandBuffer command, const vk_draw *draws, int draw_count) {
    VkViewport viewport = {0, 0, (float)context->width, (float)context->height, 0, 1};
    vkCmdSetViewport(command, 0, 1, &viewport);
    VkRect2D scissor = {{0, 0}, {(uint32_t)context->width, (uint32_t)context->height}};
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
        float push[24];
        memcpy(&push[0], draw->rect, sizeof(float) * 4);
        memcpy(&push[4], draw->uv, sizeof(float) * 4);
        memcpy(&push[8], draw->color, sizeof(float) * 4);
        push[12] = draw->opacity;
        push[13] = (float)draw->mode;
        push[14] = (float)context->width;
        push[15] = (float)context->height;
        memcpy(&push[16], draw->transform, sizeof(float) * 6);
        push[22] = (float)draw->transformed;
        push[23] = 0.0f;
        vkCmdPushConstants(command, context->pipeline_layout, VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT, 0, sizeof(push), push);
        vkCmdDraw(command, 4, 1, 0, 0);
    }
}

static vk_layer_target *create_layer_target(vk_context *context, int width, int height, char *error, size_t error_size) {
    if (context->layer_count >= context->layer_capacity) {
        int capacity = context->layer_capacity == 0 ? 8 : context->layer_capacity * 2;
        vk_layer_target *grown = realloc(context->layers, sizeof(vk_layer_target) * capacity);
        if (grown == NULL) {
            set_error(error, error_size, "out of memory for layer targets");
            return NULL;
        }
        context->layers = grown;
        context->layer_capacity = capacity;
    }
    vk_layer_target *target = &context->layers[context->layer_count];
    memset(target, 0, sizeof(vk_layer_target));
    target->width = width;
    target->height = height;
    if (!create_image(context, (uint32_t)width, (uint32_t)height, VK_FORMAT_R8G8B8A8_UNORM,
            VK_IMAGE_USAGE_COLOR_ATTACHMENT_BIT | VK_IMAGE_USAGE_SAMPLED_BIT,
            &target->image, &target->memory)) {
        set_error(error, error_size, "failed to create layer image");
        return NULL;
    }
    VkImageViewCreateInfo view_info = {0};
    view_info.sType = VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO;
    view_info.image = target->image;
    view_info.viewType = VK_IMAGE_VIEW_TYPE_2D;
    view_info.format = VK_FORMAT_R8G8B8A8_UNORM;
    view_info.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    view_info.subresourceRange.levelCount = 1;
    view_info.subresourceRange.layerCount = 1;
    if (vkCreateImageView(context->device, &view_info, NULL, &target->view) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create layer view");
        return NULL;
    }
    VkImageView attachments[1] = {target->view};
    VkFramebufferCreateInfo framebuffer_info = {0};
    framebuffer_info.sType = VK_STRUCTURE_TYPE_FRAMEBUFFER_CREATE_INFO;
    framebuffer_info.renderPass = context->layer_render_pass;
    framebuffer_info.attachmentCount = 1;
    framebuffer_info.pAttachments = attachments;
    framebuffer_info.width = (uint32_t)width;
    framebuffer_info.height = (uint32_t)height;
    framebuffer_info.layers = 1;
    if (vkCreateFramebuffer(context->device, &framebuffer_info, NULL, &target->framebuffer) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create layer framebuffer");
        return NULL;
    }
    VkDescriptorSetAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
    allocate.descriptorPool = context->descriptor_pool;
    allocate.descriptorSetCount = 1;
    allocate.pSetLayouts = &context->descriptor_layout;
    if (vkAllocateDescriptorSets(context->device, &allocate, &target->descriptor) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate layer descriptor");
        return NULL;
    }
    write_texture_descriptor(context, target->descriptor, target->view, target->view);
    if (context->texture_count >= context->texture_capacity) {
        int capacity = context->texture_capacity == 0 ? 64 : context->texture_capacity * 2;
        vk_texture *grown = realloc(context->textures, sizeof(vk_texture) * capacity);
        if (grown == NULL) {
            set_error(error, error_size, "out of memory for textures");
            return NULL;
        }
        context->textures = grown;
        context->texture_capacity = capacity;
    }
    vk_texture texture = {0};
    texture.image = target->image;
    texture.memory = target->memory;
    texture.view = target->view;
    texture.descriptor = target->descriptor;
    texture.width = width;
    texture.height = height;
    texture.format = 1;
    texture.owns_image = 0;
    context->textures[context->texture_count] = texture;
    context->texture_count++;
    target->texture_index = context->texture_count;
    context->layer_count++;
    return target;
}

int vk_context_begin_layers(vk_context *context, char *error, size_t error_size) {
    (void)error;
    (void)error_size;
    if (context == NULL) {
        return 0;
    }
    context->layer_cursor = 0;
    return 1;
}

int vk_context_render_layer(vk_context *context, int width, int height,
    const float clear[4], const vk_draw *draws, int draw_count,
    char *error, size_t error_size) {
    if (context == NULL) {
        return 0;
    }
    while (context->layer_count <= context->layer_cursor) {
        if (create_layer_target(context, width, height, error, error_size) == NULL) {
            return 0;
        }
    }
    vk_layer_target *target = &context->layers[context->layer_cursor];
    if (target->width != width || target->height != height) {
        set_error(error, error_size, "layer target size mismatch");
        return 0;
    }
    VkCommandBuffer command = begin_once(context);
    VkClearValue clear_value = {0};
    memcpy(clear_value.color.float32, clear, sizeof(float) * 4);
    VkRenderPassBeginInfo render_begin = {0};
    render_begin.sType = VK_STRUCTURE_TYPE_RENDER_PASS_BEGIN_INFO;
    render_begin.renderPass = context->layer_render_pass;
    render_begin.framebuffer = target->framebuffer;
    render_begin.renderArea.offset.x = 0;
    render_begin.renderArea.offset.y = 0;
    render_begin.renderArea.extent.width = (uint32_t)width;
    render_begin.renderArea.extent.height = (uint32_t)height;
    render_begin.clearValueCount = 1;
    render_begin.pClearValues = &clear_value;
    vkCmdBeginRenderPass(command, &render_begin, VK_SUBPASS_CONTENTS_INLINE);
    record_draws(context, command, draws, draw_count);
    vkCmdEndRenderPass(command);
    if (!end_once(context)) {
        set_error(error, error_size, "failed to render layer");
        return 0;
    }
    context->layer_cursor++;
    return target->texture_index;
}

int vk_context_pair_texture(vk_context *context, int first, int second,
    char *error, size_t error_size) {
    if (context == NULL || first <= 0 || second <= 0 ||
        first > context->texture_count || second > context->texture_count) {
        set_error(error, error_size, "invalid pair texture indices");
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
    vk_texture first_texture = context->textures[first - 1];
    vk_texture second_texture = context->textures[second - 1];
    VkDescriptorSet descriptor = VK_NULL_HANDLE;
    VkDescriptorSetAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
    allocate.descriptorPool = context->descriptor_pool;
    allocate.descriptorSetCount = 1;
    allocate.pSetLayouts = &context->descriptor_layout;
    if (vkAllocateDescriptorSets(context->device, &allocate, &descriptor) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate pair descriptor");
        return 0;
    }
    write_texture_descriptor(context, descriptor, first_texture.view, second_texture.view);
    vk_texture texture = {0};
    texture.image = first_texture.image;
    texture.memory = first_texture.memory;
    texture.view = first_texture.view;
    texture.descriptor = descriptor;
    texture.width = first_texture.width;
    texture.height = first_texture.height;
    texture.format = first_texture.format;
    texture.owns_image = 0;
    context->textures[context->texture_count] = texture;
    context->texture_count++;
    return context->texture_count;
}

int vk_context_create_path_pipeline(vk_context *context, const uint32_t *code, size_t size, char *error, size_t error_size) {
    if (context == NULL || code == NULL || size == 0) {
        set_error(error, error_size, "invalid path shader");
        return 0;
    }
    VkShaderModule module = create_shader_module(context, code, size);
    if (module == VK_NULL_HANDLE) {
        set_error(error, error_size, "failed to create path shader module");
        return 0;
    }
    VkDescriptorSetLayoutBinding bindings[2] = {0};
    bindings[0].binding = 0;
    bindings[0].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    bindings[0].descriptorCount = 1;
    bindings[0].stageFlags = VK_SHADER_STAGE_COMPUTE_BIT;
    bindings[1].binding = 1;
    bindings[1].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_IMAGE;
    bindings[1].descriptorCount = 1;
    bindings[1].stageFlags = VK_SHADER_STAGE_COMPUTE_BIT;
    VkDescriptorSetLayoutCreateInfo layout_info = {0};
    layout_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO;
    layout_info.bindingCount = 2;
    layout_info.pBindings = bindings;
    if (vkCreateDescriptorSetLayout(context->device, &layout_info, NULL, &context->path_set_layout) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create path descriptor layout");
        return 0;
    }
    VkDescriptorPoolSize pool_sizes[2] = {0};
    pool_sizes[0].type = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    pool_sizes[0].descriptorCount = 1;
    pool_sizes[1].type = VK_DESCRIPTOR_TYPE_STORAGE_IMAGE;
    pool_sizes[1].descriptorCount = 1;
    VkDescriptorPoolCreateInfo pool_info = {0};
    pool_info.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO;
    pool_info.maxSets = 1;
    pool_info.poolSizeCount = 2;
    pool_info.pPoolSizes = pool_sizes;
    if (vkCreateDescriptorPool(context->device, &pool_info, NULL, &context->path_descriptor_pool) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create path descriptor pool");
        return 0;
    }
    VkDescriptorSetAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
    allocate.descriptorPool = context->path_descriptor_pool;
    allocate.descriptorSetCount = 1;
    allocate.pSetLayouts = &context->path_set_layout;
    if (vkAllocateDescriptorSets(context->device, &allocate, &context->path_descriptor) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate path descriptor");
        return 0;
    }
    VkPushConstantRange range = {0};
    range.stageFlags = VK_SHADER_STAGE_COMPUTE_BIT;
    range.offset = 0;
    range.size = sizeof(int) * 4;
    VkPipelineLayoutCreateInfo pipeline_layout_info = {0};
    pipeline_layout_info.sType = VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO;
    pipeline_layout_info.setLayoutCount = 1;
    pipeline_layout_info.pSetLayouts = &context->path_set_layout;
    pipeline_layout_info.pushConstantRangeCount = 1;
    pipeline_layout_info.pPushConstantRanges = &range;
    if (vkCreatePipelineLayout(context->device, &pipeline_layout_info, NULL, &context->path_pipeline_layout) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create path pipeline layout");
        return 0;
    }
    VkComputePipelineCreateInfo pipeline_info = {0};
    pipeline_info.sType = VK_STRUCTURE_TYPE_COMPUTE_PIPELINE_CREATE_INFO;
    pipeline_info.stage.sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    pipeline_info.stage.stage = VK_SHADER_STAGE_COMPUTE_BIT;
    pipeline_info.stage.module = module;
    pipeline_info.stage.pName = "main";
    pipeline_info.layout = context->path_pipeline_layout;
    if (vkCreateComputePipelines(context->device, VK_NULL_HANDLE, 1, &pipeline_info, NULL, &context->path_pipeline) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create path pipeline");
        vkDestroyShaderModule(context->device, module, NULL);
        return 0;
    }
    vkDestroyShaderModule(context->device, module, NULL);
    return 1;
}

static int ensure_storage_buffer(vk_context *context, VkBuffer *buffer, VkDeviceMemory *memory,
    VkDeviceSize *capacity, VkDeviceSize required, VkBufferUsageFlags usage,
    VkMemoryPropertyFlags properties, char *error, size_t error_size) {
    if (*capacity >= required && *buffer != VK_NULL_HANDLE) {
        return 1;
    }
    if (*buffer != VK_NULL_HANDLE) {
        vkDestroyBuffer(context->device, *buffer, NULL);
        vkFreeMemory(context->device, *memory, NULL);
        *buffer = VK_NULL_HANDLE;
        *memory = VK_NULL_HANDLE;
        *capacity = 0;
    }
    if (!create_buffer(context, required, usage, properties, buffer, memory)) {
        set_error(error, error_size, "failed to create path storage buffer");
        return 0;
    }
    *capacity = required;
    return 1;
}

int vk_context_rasterize_path(vk_context *context, const float *edges, int edge_count,
    int width, int height, int rule, char *error, size_t error_size) {
    if (context == NULL || context->path_pipeline == VK_NULL_HANDLE || edge_count <= 0 ||
        width <= 0 || height <= 0) {
        set_error(error, error_size, "invalid path raster request");
        return 0;
    }
    VkDeviceSize edge_bytes = (VkDeviceSize)edge_count * 5 * sizeof(float);
    if (!ensure_storage_buffer(context, &context->edge_buffer, &context->edge_memory,
            &context->edge_capacity, edge_bytes, VK_BUFFER_USAGE_STORAGE_BUFFER_BIT,
            VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT,
            error, error_size)) {
        return 0;
    }
    void *mapped = NULL;
    if (vkMapMemory(context->device, context->edge_memory, 0, edge_bytes, 0, &mapped) != VK_SUCCESS) {
        set_error(error, error_size, "failed to map edge buffer");
        return 0;
    }
    memcpy(mapped, edges, (size_t)edge_bytes);
    vkUnmapMemory(context->device, context->edge_memory);

    VkImage coverage_image = VK_NULL_HANDLE;
    VkDeviceMemory coverage_memory = VK_NULL_HANDLE;
    VkImageView coverage_view = VK_NULL_HANDLE;
    if (!create_image(context, (uint32_t)width, (uint32_t)height, VK_FORMAT_R8G8B8A8_UNORM,
            VK_IMAGE_USAGE_STORAGE_BIT | VK_IMAGE_USAGE_SAMPLED_BIT,
            &coverage_image, &coverage_memory)) {
        set_error(error, error_size, "failed to create coverage image");
        return 0;
    }
    VkImageViewCreateInfo view_info = {0};
    view_info.sType = VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO;
    view_info.image = coverage_image;
    view_info.viewType = VK_IMAGE_VIEW_TYPE_2D;
    view_info.format = VK_FORMAT_R8G8B8A8_UNORM;
    view_info.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    view_info.subresourceRange.levelCount = 1;
    view_info.subresourceRange.layerCount = 1;
    if (vkCreateImageView(context->device, &view_info, NULL, &coverage_view) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create coverage view");
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
    texture.image = coverage_image;
    texture.memory = coverage_memory;
    texture.view = coverage_view;
    texture.width = width;
    texture.height = height;
    texture.format = 1;
    texture.owns_image = 1;
    VkDescriptorSetAllocateInfo allocate = {0};
    allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
    allocate.descriptorPool = context->descriptor_pool;
    allocate.descriptorSetCount = 1;
    allocate.pSetLayouts = &context->descriptor_layout;
    if (vkAllocateDescriptorSets(context->device, &allocate, &texture.descriptor) != VK_SUCCESS) {
        set_error(error, error_size, "failed to allocate coverage descriptor");
        return 0;
    }
    write_texture_descriptor(context, texture.descriptor, coverage_view, coverage_view);
    context->textures[context->texture_count] = texture;
    context->texture_count++;
    context->coverage_texture_index = context->texture_count;
    context->coverage_width = width;
    context->coverage_height = height;

    VkDescriptorBufferInfo edge_info = {0};
    edge_info.buffer = context->edge_buffer;
    edge_info.offset = 0;
    edge_info.range = edge_bytes;
    VkDescriptorImageInfo image_info = {0};
    image_info.imageLayout = VK_IMAGE_LAYOUT_GENERAL;
    image_info.imageView = coverage_view;
    VkWriteDescriptorSet writes[2] = {0};
    writes[0].sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
    writes[0].dstSet = context->path_descriptor;
    writes[0].dstBinding = 0;
    writes[0].descriptorCount = 1;
    writes[0].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    writes[0].pBufferInfo = &edge_info;
    writes[1].sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
    writes[1].dstSet = context->path_descriptor;
    writes[1].dstBinding = 1;
    writes[1].descriptorCount = 1;
    writes[1].descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_IMAGE;
    writes[1].pImageInfo = &image_info;
    vkUpdateDescriptorSets(context->device, 2, writes, 0, NULL);

    VkCommandBuffer command = begin_once(context);
    image_barrier(command, coverage_image, VK_IMAGE_LAYOUT_UNDEFINED,
        VK_IMAGE_LAYOUT_GENERAL, 0, VK_ACCESS_SHADER_WRITE_BIT,
        VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT, VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT);
    vkCmdBindPipeline(command, VK_PIPELINE_BIND_POINT_COMPUTE, context->path_pipeline);
    vkCmdBindDescriptorSets(command, VK_PIPELINE_BIND_POINT_COMPUTE, context->path_pipeline_layout, 0, 1, &context->path_descriptor, 0, NULL);
    int push[4] = {edge_count, width, height, rule};
    vkCmdPushConstants(command, context->path_pipeline_layout, VK_SHADER_STAGE_COMPUTE_BIT, 0, sizeof(push), push);
    vkCmdDispatch(command, (uint32_t)((width + 7) / 8), (uint32_t)((height + 7) / 8), 1);
    image_barrier(command, coverage_image, VK_IMAGE_LAYOUT_GENERAL,
        VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL, VK_ACCESS_SHADER_WRITE_BIT,
        VK_ACCESS_SHADER_READ_BIT, VK_PIPELINE_STAGE_COMPUTE_SHADER_BIT,
        VK_PIPELINE_STAGE_FRAGMENT_SHADER_BIT);
    if (!end_once(context)) {
        set_error(error, error_size, "failed to rasterize path");
        return 0;
    }
    return context->coverage_texture_index;
}

static int create_swapchain(vk_context *context, int width, int height, char *error, size_t error_size) {
    if (context->swapchain != VK_NULL_HANDLE) {
        for (uint32_t index = 0; index < context->swapchain_image_count; index++) {
            if (context->swapchain_views[index] != VK_NULL_HANDLE) {
                vkDestroyImageView(context->device, context->swapchain_views[index], NULL);
            }
        }
        free(context->swapchain_views);
        free(context->swapchain_images);
        context->pDestroySwapchain(context->device, context->swapchain, NULL);
        context->swapchain = VK_NULL_HANDLE;
    }
    VkSurfaceCapabilitiesKHR capabilities;
    if (context->pGetSurfaceCapabilities(context->physical_device, context->surface, &capabilities) != VK_SUCCESS) {
        set_error(error, error_size, "failed to query surface capabilities");
        return 0;
    }
    uint32_t format_count = 0;
    context->pGetSurfaceFormats(context->physical_device, context->surface, &format_count, NULL);
    VkSurfaceFormatKHR *formats = calloc(format_count, sizeof(VkSurfaceFormatKHR));
    context->pGetSurfaceFormats(context->physical_device, context->surface, &format_count, formats);
    VkSurfaceFormatKHR chosen = {0};
    int found = 0;
    for (uint32_t index = 0; index < format_count; index++) {
        if (formats[index].format == VK_FORMAT_R8G8B8A8_UNORM) {
            chosen = formats[index];
            context->swapchain_bgra = 0;
            found = 1;
            break;
        }
    }
    if (!found) {
        for (uint32_t index = 0; index < format_count; index++) {
            if (formats[index].format == VK_FORMAT_B8G8R8A8_UNORM) {
                chosen = formats[index];
                context->swapchain_bgra = 1;
                found = 1;
                break;
            }
        }
    }
    if (!found && format_count > 0) {
        chosen = formats[0];
        context->swapchain_bgra = chosen.format == VK_FORMAT_B8G8R8A8_UNORM;
    }
    free(formats);
    if (!found && format_count == 0) {
        set_error(error, error_size, "no surface formats");
        return 0;
    }
    VkExtent2D extent = capabilities.currentExtent;
    if (extent.width == UINT32_MAX) {
        extent.width = (uint32_t)width;
        extent.height = (uint32_t)height;
    }
    uint32_t image_count = capabilities.minImageCount + 1;
    if (capabilities.maxImageCount > 0 && image_count > capabilities.maxImageCount) {
        image_count = capabilities.maxImageCount;
    }
    VkSwapchainCreateInfoKHR swapchain_info = {0};
    swapchain_info.sType = VK_STRUCTURE_TYPE_SWAPCHAIN_CREATE_INFO_KHR;
    swapchain_info.surface = context->surface;
    swapchain_info.minImageCount = image_count;
    swapchain_info.imageFormat = chosen.format;
    swapchain_info.imageColorSpace = chosen.colorSpace;
    swapchain_info.imageExtent = extent;
    swapchain_info.imageArrayLayers = 1;
    swapchain_info.imageUsage = VK_IMAGE_USAGE_TRANSFER_DST_BIT;
    swapchain_info.imageSharingMode = VK_SHARING_MODE_EXCLUSIVE;
    swapchain_info.preTransform = capabilities.currentTransform;
    swapchain_info.compositeAlpha = VK_COMPOSITE_ALPHA_OPAQUE_BIT_KHR;
    swapchain_info.presentMode = VK_PRESENT_MODE_FIFO_KHR;
    swapchain_info.clipped = VK_TRUE;
    if (context->pCreateSwapchain(context->device, &swapchain_info, NULL, &context->swapchain) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create swapchain");
        return 0;
    }
    context->pGetSwapchainImages(context->device, context->swapchain, &context->swapchain_image_count, NULL);
    context->swapchain_images = calloc(context->swapchain_image_count, sizeof(VkImage));
    context->swapchain_views = calloc(context->swapchain_image_count, sizeof(VkImageView));
    context->pGetSwapchainImages(context->device, context->swapchain, &context->swapchain_image_count, context->swapchain_images);
    for (uint32_t index = 0; index < context->swapchain_image_count; index++) {
        VkImageViewCreateInfo view_info = {0};
        view_info.sType = VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO;
        view_info.image = context->swapchain_images[index];
        view_info.viewType = VK_IMAGE_VIEW_TYPE_2D;
        view_info.format = chosen.format;
        view_info.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
        view_info.subresourceRange.levelCount = 1;
        view_info.subresourceRange.layerCount = 1;
        if (vkCreateImageView(context->device, &view_info, NULL, &context->swapchain_views[index]) != VK_SUCCESS) {
            set_error(error, error_size, "failed to create swapchain view");
            return 0;
        }
    }
    context->swapchain_format = chosen.format;
    context->swapchain_extent = extent;
    return 1;
}

int vk_context_attach_swapchain(vk_context *context, const char *display, uintptr_t window,
    int width, int height, char *error, size_t error_size) {
    if (context == NULL || window == 0) {
        set_error(error, error_size, "invalid swapchain window");
        return 0;
    }
    context->xcb_connection = xcb_connect(display, NULL);
    if (context->xcb_connection == NULL || xcb_connection_has_error(context->xcb_connection)) {
        set_error(error, error_size, "failed to connect to X server");
        return 0;
    }
    PFN_vkCreateXcbSurfaceKHR create_surface = (PFN_vkCreateXcbSurfaceKHR)vkGetInstanceProcAddr(context->instance, "vkCreateXcbSurfaceKHR");
    if (create_surface == NULL) {
        set_error(error, error_size, "VK_KHR_xcb_surface unavailable");
        return 0;
    }
    VkXcbSurfaceCreateInfoKHR surface_info = {0};
    surface_info.sType = VK_STRUCTURE_TYPE_XCB_SURFACE_CREATE_INFO_KHR;
    surface_info.connection = context->xcb_connection;
    surface_info.window = (xcb_window_t)window;
    if (create_surface(context->instance, &surface_info, NULL, &context->surface) != VK_SUCCESS) {
        set_error(error, error_size, "failed to create XCB surface");
        return 0;
    }
    context->pGetSurfaceCapabilities = (PFN_vkGetPhysicalDeviceSurfaceCapabilitiesKHR)vkGetInstanceProcAddr(context->instance, "vkGetPhysicalDeviceSurfaceCapabilitiesKHR");
    context->pGetSurfaceFormats = (PFN_vkGetPhysicalDeviceSurfaceFormatsKHR)vkGetInstanceProcAddr(context->instance, "vkGetPhysicalDeviceSurfaceFormatsKHR");
    context->pGetSurfacePresentModes = (PFN_vkGetPhysicalDeviceSurfacePresentModesKHR)vkGetInstanceProcAddr(context->instance, "vkGetPhysicalDeviceSurfacePresentModesKHR");
    context->pCreateSwapchain = (PFN_vkCreateSwapchainKHR)vkGetDeviceProcAddr(context->device, "vkCreateSwapchainKHR");
    context->pDestroySwapchain = (PFN_vkDestroySwapchainKHR)vkGetDeviceProcAddr(context->device, "vkDestroySwapchainKHR");
    context->pGetSwapchainImages = (PFN_vkGetSwapchainImagesKHR)vkGetDeviceProcAddr(context->device, "vkGetSwapchainImagesKHR");
    context->pAcquireNextImage = (PFN_vkAcquireNextImageKHR)vkGetDeviceProcAddr(context->device, "vkAcquireNextImageKHR");
    context->pQueuePresent = (PFN_vkQueuePresentKHR)vkGetDeviceProcAddr(context->device, "vkQueuePresentKHR");
    if (context->pGetSurfaceCapabilities == NULL || context->pGetSurfaceFormats == NULL ||
        context->pCreateSwapchain == NULL || context->pQueuePresent == NULL) {
        set_error(error, error_size, "swapchain functions unavailable");
        return 0;
    }
    VkSemaphoreCreateInfo semaphore_info = {0};
    semaphore_info.sType = VK_STRUCTURE_TYPE_SEMAPHORE_CREATE_INFO;
    vkCreateSemaphore(context->device, &semaphore_info, NULL, &context->image_available);
    vkCreateSemaphore(context->device, &semaphore_info, NULL, &context->render_finished);
    return create_swapchain(context, width, height, error, error_size);
}

int vk_context_present_surface(vk_context *context, const uint8_t *pixels,
    int width, int height, char *error, size_t error_size) {
    if (context == NULL || context->swapchain == VK_NULL_HANDLE || pixels == NULL) {
        set_error(error, error_size, "swapchain unavailable");
        return 0;
    }
    VkDeviceSize byte_size = (VkDeviceSize)width * height * 4;
    if (!ensure_storage_buffer(context, &context->present_buffer, &context->present_memory,
            &context->present_capacity, byte_size, VK_BUFFER_USAGE_TRANSFER_SRC_BIT,
            VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT,
            error, error_size)) {
        return 0;
    }
    void *mapped = NULL;
    if (vkMapMemory(context->device, context->present_memory, 0, byte_size, 0, &mapped) != VK_SUCCESS) {
        set_error(error, error_size, "failed to map present buffer");
        return 0;
    }
    if (context->swapchain_bgra) {
        uint8_t *target = (uint8_t *)mapped;
        for (VkDeviceSize index = 0; index < byte_size; index += 4) {
            target[index] = pixels[index + 2];
            target[index + 1] = pixels[index + 1];
            target[index + 2] = pixels[index];
            target[index + 3] = pixels[index + 3];
        }
    } else {
        memcpy(mapped, pixels, (size_t)byte_size);
    }
    vkUnmapMemory(context->device, context->present_memory);
    uint32_t image_index = 0;
    VkResult acquire = context->pAcquireNextImage(context->device, context->swapchain, UINT64_MAX,
        context->image_available, VK_NULL_HANDLE, &image_index);
    if (acquire == VK_ERROR_OUT_OF_DATE_KHR) {
        set_error(error, error_size, "swapchain out of date");
        return 0;
    }
    VkCommandBuffer command = begin_once(context);
    image_barrier(command, context->swapchain_images[image_index], VK_IMAGE_LAYOUT_UNDEFINED,
        VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 0, VK_ACCESS_TRANSFER_WRITE_BIT,
        VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT, VK_PIPELINE_STAGE_TRANSFER_BIT);
    VkBufferImageCopy region = {0};
    region.imageSubresource.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
    region.imageSubresource.layerCount = 1;
    region.imageExtent.width = (uint32_t)width;
    region.imageExtent.height = (uint32_t)height;
    region.imageExtent.depth = 1;
    vkCmdCopyBufferToImage(command, context->present_buffer, context->swapchain_images[image_index],
        VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 1, &region);
    image_barrier(command, context->swapchain_images[image_index], VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL,
        VK_IMAGE_LAYOUT_PRESENT_SRC_KHR, VK_ACCESS_TRANSFER_WRITE_BIT, 0,
        VK_PIPELINE_STAGE_TRANSFER_BIT, VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT);
    vkEndCommandBuffer(command);
    VkPipelineStageFlags wait_stage = VK_PIPELINE_STAGE_TRANSFER_BIT;
    VkSubmitInfo submit = {0};
    submit.sType = VK_STRUCTURE_TYPE_SUBMIT_INFO;
    submit.waitSemaphoreCount = 1;
    submit.pWaitSemaphores = &context->image_available;
    submit.pWaitDstStageMask = &wait_stage;
    submit.commandBufferCount = 1;
    submit.pCommandBuffers = &command;
    submit.signalSemaphoreCount = 1;
    submit.pSignalSemaphores = &context->render_finished;
    vkResetFences(context->device, 1, &context->fence);
    if (vkQueueSubmit(context->queue, 1, &submit, context->fence) != VK_SUCCESS) {
        set_error(error, error_size, "failed to submit present");
        return 0;
    }
    vkWaitForFences(context->device, 1, &context->fence, VK_TRUE, UINT64_MAX);
    VkPresentInfoKHR present = {0};
    present.sType = VK_STRUCTURE_TYPE_PRESENT_INFO_KHR;
    present.waitSemaphoreCount = 1;
    present.pWaitSemaphores = &context->render_finished;
    present.swapchainCount = 1;
    present.pSwapchains = &context->swapchain;
    present.pImageIndices = &image_index;
    if (context->pQueuePresent(context->queue, &present) != VK_SUCCESS) {
        set_error(error, error_size, "failed to present");
        return 0;
    }
    return 1;
}

int vk_context_geometry_texture(vk_context *context, uintptr_t buffer, char *error, size_t error_size) {
    if (context == NULL || buffer == 0) {
        set_error(error, error_size, "invalid geometry buffer");
        return 0;
    }
    if (context->geometry_texture_index == 0) {
        if (!create_image(context, 1, 1, VK_FORMAT_R8G8B8A8_UNORM, VK_IMAGE_USAGE_SAMPLED_BIT,
                &context->geometry_image, &context->geometry_image_memory)) {
            set_error(error, error_size, "failed to create geometry image");
            return 0;
        }
        VkImageViewCreateInfo view_info = {0};
        view_info.sType = VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO;
        view_info.image = context->geometry_image;
        view_info.viewType = VK_IMAGE_VIEW_TYPE_2D;
        view_info.format = VK_FORMAT_R8G8B8A8_UNORM;
        view_info.subresourceRange.aspectMask = VK_IMAGE_ASPECT_COLOR_BIT;
        view_info.subresourceRange.levelCount = 1;
        view_info.subresourceRange.layerCount = 1;
        if (vkCreateImageView(context->device, &view_info, NULL, &context->geometry_image_view) != VK_SUCCESS) {
            set_error(error, error_size, "failed to create geometry view");
            return 0;
        }
        VkDescriptorSetAllocateInfo allocate = {0};
        allocate.sType = VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO;
        allocate.descriptorPool = context->descriptor_pool;
        allocate.descriptorSetCount = 1;
        allocate.pSetLayouts = &context->descriptor_layout;
        if (vkAllocateDescriptorSets(context->device, &allocate, &context->geometry_descriptor) != VK_SUCCESS) {
            set_error(error, error_size, "failed to allocate geometry descriptor");
            return 0;
        }
        write_texture_descriptor(context, context->geometry_descriptor, context->geometry_image_view, context->geometry_image_view);
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
        texture.image = context->geometry_image;
        texture.memory = context->geometry_image_memory;
        texture.view = context->geometry_image_view;
        texture.descriptor = context->geometry_descriptor;
        texture.width = 1;
        texture.height = 1;
        texture.format = 1;
        texture.owns_image = 0;
        context->textures[context->texture_count] = texture;
        context->texture_count++;
        context->geometry_texture_index = context->texture_count;
    }
    VkDescriptorBufferInfo buffer_info = {0};
    buffer_info.buffer = (VkBuffer)buffer;
    buffer_info.offset = 0;
    buffer_info.range = VK_WHOLE_SIZE;
    VkWriteDescriptorSet write = {0};
    write.sType = VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET;
    write.dstSet = context->geometry_descriptor;
    write.dstBinding = 2;
    write.descriptorCount = 1;
    write.descriptorType = VK_DESCRIPTOR_TYPE_STORAGE_BUFFER;
    write.pBufferInfo = &buffer_info;
    vkUpdateDescriptorSets(context->device, 1, &write, 0, NULL);
    return context->geometry_texture_index;
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
        float push[24];
        memcpy(&push[0], draw->rect, sizeof(float) * 4);
        memcpy(&push[4], draw->uv, sizeof(float) * 4);
        memcpy(&push[8], draw->color, sizeof(float) * 4);
        push[12] = draw->opacity;
        push[13] = (float)draw->mode;
        push[14] = (float)context->width;
        push[15] = (float)context->height;
        memcpy(&push[16], draw->transform, sizeof(float) * 6);
        push[22] = (float)draw->transformed;
        push[23] = 0.0f;
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
            if (!texture->owns_image) {
                continue;
            }
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
        for (int index = 0; index < context->layer_count; index++) {
            vk_layer_target *target = &context->layers[index];
            if (target->framebuffer != VK_NULL_HANDLE) vkDestroyFramebuffer(context->device, target->framebuffer, NULL);
            if (target->view != VK_NULL_HANDLE) vkDestroyImageView(context->device, target->view, NULL);
            if (target->image != VK_NULL_HANDLE) vkDestroyImage(context->device, target->image, NULL);
            if (target->memory != VK_NULL_HANDLE) vkFreeMemory(context->device, target->memory, NULL);
        }
        free(context->layers);
        if (context->solid_pipeline != VK_NULL_HANDLE) vkDestroyPipeline(context->device, context->solid_pipeline, NULL);
        if (context->texture_pipeline != VK_NULL_HANDLE) vkDestroyPipeline(context->device, context->texture_pipeline, NULL);
        if (context->pipeline_layout != VK_NULL_HANDLE) vkDestroyPipelineLayout(context->device, context->pipeline_layout, NULL);
        if (context->descriptor_pool != VK_NULL_HANDLE) vkDestroyDescriptorPool(context->device, context->descriptor_pool, NULL);
        if (context->descriptor_layout != VK_NULL_HANDLE) vkDestroyDescriptorSetLayout(context->device, context->descriptor_layout, NULL);
        if (context->sampler != VK_NULL_HANDLE) vkDestroySampler(context->device, context->sampler, NULL);
        if (context->framebuffer != VK_NULL_HANDLE) vkDestroyFramebuffer(context->device, context->framebuffer, NULL);
        if (context->swapchain != VK_NULL_HANDLE) {
            for (uint32_t index = 0; index < context->swapchain_image_count; index++) {
                if (context->swapchain_views[index] != VK_NULL_HANDLE) vkDestroyImageView(context->device, context->swapchain_views[index], NULL);
            }
            free(context->swapchain_views);
            free(context->swapchain_images);
            context->pDestroySwapchain(context->device, context->swapchain, NULL);
        }
        if (context->swapchain != VK_NULL_HANDLE && context->surface != VK_NULL_HANDLE) {
            vkDestroySurfaceKHR(context->instance, context->surface, NULL);
            context->surface = VK_NULL_HANDLE;
        }
        if (context->image_available != VK_NULL_HANDLE) vkDestroySemaphore(context->device, context->image_available, NULL);
        if (context->render_finished != VK_NULL_HANDLE) vkDestroySemaphore(context->device, context->render_finished, NULL);
        if (context->present_buffer != VK_NULL_HANDLE) vkDestroyBuffer(context->device, context->present_buffer, NULL);
        if (context->present_memory != VK_NULL_HANDLE) vkFreeMemory(context->device, context->present_memory, NULL);
        if (context->xcb_connection != NULL) xcb_disconnect(context->xcb_connection);
        if (context->geometry_image_view != VK_NULL_HANDLE) vkDestroyImageView(context->device, context->geometry_image_view, NULL);
        if (context->geometry_image != VK_NULL_HANDLE) vkDestroyImage(context->device, context->geometry_image, NULL);
        if (context->geometry_image_memory != VK_NULL_HANDLE) vkFreeMemory(context->device, context->geometry_image_memory, NULL);
        if (context->dummy_node_buffer != VK_NULL_HANDLE) vkDestroyBuffer(context->device, context->dummy_node_buffer, NULL);
        if (context->dummy_node_memory != VK_NULL_HANDLE) vkFreeMemory(context->device, context->dummy_node_memory, NULL);
        if (context->layer_render_pass != VK_NULL_HANDLE) vkDestroyRenderPass(context->device, context->layer_render_pass, NULL);
        if (context->render_pass != VK_NULL_HANDLE) vkDestroyRenderPass(context->device, context->render_pass, NULL);
        if (context->image_view != VK_NULL_HANDLE) vkDestroyImageView(context->device, context->image_view, NULL);
        if (context->image != VK_NULL_HANDLE) vkDestroyImage(context->device, context->image, NULL);
        if (context->image_memory != VK_NULL_HANDLE) vkFreeMemory(context->device, context->image_memory, NULL);
        if (context->fence != VK_NULL_HANDLE) vkDestroyFence(context->device, context->fence, NULL);
        if (context->command_pool != VK_NULL_HANDLE) vkDestroyCommandPool(context->device, context->command_pool, NULL);
        if (context->owns_device) {
            vkDestroyDevice(context->device, NULL);
        }
    }
    if (context->owns_device && context->instance != VK_NULL_HANDLE) {
        vkDestroyInstance(context->instance, NULL);
    }
    free(context);
}
