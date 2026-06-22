/**
 * WebGL2 video renderer.
 *
 * Uploads a decoded `VideoFrame` to a texture and draws it to a fullscreen
 * triangle. This replaces the 2D-canvas `drawImage(VideoFrame)` path.
 *
 * Why: on Safari, `CanvasRenderingContext2D.drawImage(VideoFrame)` at 4K is not
 * GPU-accelerated — it does a per-frame CPU copy + colorspace convert of a ~33MB
 * surface, which capped presentation at single-digit fps even though the hardware
 * decoder and the main thread were idle (the UI stayed responsive). Chrome
 * accelerates both paths, but WebGL2 `texImage2D` from a VideoFrame is the
 * portable fast path that every modern engine GPU-accelerates, and it also
 * sidesteps the `desynchronized` 2D-canvas presentation throttle.
 *
 * The frame is YUV (NV12) from the hardware decoder; the browser performs the
 * YUV→RGB conversion as part of the texImage2D upload, honouring the frame's
 * color space. No vertex buffers are needed — the fullscreen triangle is
 * generated from `gl_VertexID`.
 */

const VERTEX_SHADER = `#version 300 es
out vec2 v_uv;
void main() {
  // Fullscreen triangle from gl_VertexID (vertices 0,1,2) — no vertex buffers.
  vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  // Flip V: VideoFrame rows are top-down, GL texture origin is bottom-left.
  v_uv = vec2(p.x, 1.0 - p.y);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`

const FRAGMENT_SHADER = `#version 300 es
precision mediump float;
in vec2 v_uv;
uniform sampler2D u_tex;
out vec4 o_color;
void main() {
  o_color = texture(u_tex, v_uv);
}`

export class WebGLVideoRenderer {
  private gl: WebGL2RenderingContext
  private program: WebGLProgram
  private texture: WebGLTexture
  private vao: WebGLVertexArrayObject
  private vpW = 0
  private vpH = 0

  constructor(private canvas: HTMLCanvasElement) {
    const gl = canvas.getContext("webgl2", {
      alpha: false,
      antialias: false,
      depth: false,
      stencil: false,
      premultipliedAlpha: false,
      preserveDrawingBuffer: false,
      powerPreference: "high-performance",
    })
    if (!gl) {
      throw new Error("WebGL2 not available")
    }
    this.gl = gl

    this.program = this.buildProgram(VERTEX_SHADER, FRAGMENT_SHADER)

    // A bound VAO is required to draw in WebGL2 even with no vertex attributes.
    this.vao = gl.createVertexArray()!
    gl.bindVertexArray(this.vao)

    this.texture = gl.createTexture()!
    gl.bindTexture(gl.TEXTURE_2D, this.texture)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

    gl.useProgram(this.program)
    gl.uniform1i(gl.getUniformLocation(this.program, "u_tex"), 0)
    gl.activeTexture(gl.TEXTURE0)
  }

  /** Upload one frame and present it. Resizes the canvas/viewport on change. */
  draw(frame: VideoFrame, targetWidth: number, targetHeight: number) {
    const gl = this.gl
    if (this.canvas.width !== targetWidth || this.canvas.height !== targetHeight) {
      this.canvas.width = targetWidth
      this.canvas.height = targetHeight
    }
    if (this.vpW !== targetWidth || this.vpH !== targetHeight) {
      this.vpW = targetWidth
      this.vpH = targetHeight
      gl.viewport(0, 0, targetWidth, targetHeight)
    }
    gl.bindTexture(gl.TEXTURE_2D, this.texture)
    // Re-spec the texture each frame so resolution changes mid-stream are handled.
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, frame)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
  }

  dispose() {
    const gl = this.gl
    try {
      gl.deleteTexture(this.texture)
      gl.deleteProgram(this.program)
      gl.deleteVertexArray(this.vao)
      gl.getExtension("WEBGL_lose_context")?.loseContext()
    } catch {
      /* noop */
    }
  }

  private buildProgram(vsSrc: string, fsSrc: string): WebGLProgram {
    const gl = this.gl
    const vs = this.compile(gl.VERTEX_SHADER, vsSrc)
    const fs = this.compile(gl.FRAGMENT_SHADER, fsSrc)
    const program = gl.createProgram()!
    gl.attachShader(program, vs)
    gl.attachShader(program, fs)
    gl.linkProgram(program)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      const log = gl.getProgramInfoLog(program)
      gl.deleteProgram(program)
      throw new Error(`WebGL program link failed: ${log}`)
    }
    gl.deleteShader(vs)
    gl.deleteShader(fs)
    return program
  }

  private compile(type: number, src: string): WebGLShader {
    const gl = this.gl
    const shader = gl.createShader(type)!
    gl.shaderSource(shader, src)
    gl.compileShader(shader)
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      const log = gl.getShaderInfoLog(shader)
      gl.deleteShader(shader)
      throw new Error(`WebGL shader compile failed: ${log}`)
    }
    return shader
  }
}
