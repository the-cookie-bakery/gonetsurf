// Package rendering is the paint frontend: it turns laid-out boxes and
// computed styles into a backend-neutral scene using interfaces.Canvas.
//
// Concrete rendering pipelines live in the software and Vulkan subpackages;
// shared value types live in rendering/geometry and backend contracts in
// rendering/interfaces.
package rendering
