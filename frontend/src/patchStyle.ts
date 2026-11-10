// How a NetLogo patch is drawn in the 3D view.
//
// Desktop NetLogo 3D does not fill the world with opaque cubes. Black
// patches are absent, and the patches that remain are translucent so
// turtles inside the volume stay visible. A color number is fully opaque
// in 2D; here it is drawn at solidPatchOpacity. An RGBA color keeps the
// alpha the model asked for. The cube is smaller than a patch so a gap
// shows between neighbors instead of a solid block.

export const patchCube = 0.78
export const solidPatchOpacity = 0.45

// Bytes in one packed voxel: int16 x, y, z, then r, g, b, a.
export const voxelStride = 10

// patchOpacity is the instance alpha for a packed color alpha byte.
// Zero and negative values draw nothing. 255 is a normal NetLogo color.
export function patchOpacity(alpha: number): number {
  if (!Number.isFinite(alpha) || alpha <= 0) return 0
  if (alpha >= 255) return solidPatchOpacity
  return alpha / 255
}
