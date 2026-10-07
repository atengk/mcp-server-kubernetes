/**
 * 封装跨平台架构矩阵映射与原生二进制路径探测解析器。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const __dirname = path.dirname(fileURLToPath(import.meta.url));

/**
 * 声明官方受支持的五大主流目标操作系统平台与硬件芯片架构矩阵。
 */
export const SUPPORTED_MATRIX = [
  'darwin-arm64',
  'darwin-x64',
  'linux-x64',
  'linux-arm64',
  'win32-x64',
];

/**
 * 二进制可执行文件基础文件名。
 */
export const BINARY_BASENAME = 'mcp-server-kubernetes';

/**
 * 组织命名空间前缀。
 */
export const SCOPE_PREFIX = '@atengk/mcp-server-kubernetes-';

/**
 * 计算并校验当前宿主机平台与芯片架构的标准 Matrix 标识。
 *
 * @param {string} platform 操作系统平台 (如 process.platform)
 * @param {string} arch 硬件芯片架构 (如 process.arch)
 * @returns {string} 标准矩阵键名 (如 linux-x64)
 * @throws {Error} 若当前平台与芯片架构不受支持
 */
export function getPlatformArchKey(platform, arch) {
  const key = `${platform}-${arch}`;
  if (!SUPPORTED_MATRIX.includes(key)) {
    throw new Error(
      `不支持的操作系统平台或芯片架构: ${key}。\n支持的平台矩阵包括: ${SUPPORTED_MATRIX.join(', ')}`
    );
  }
  return key;
}

/**
 * 获取指定平台架构对应的 npm 专属子包全称。
 *
 * @param {string} key 平台矩阵键名 (如 win32-x64)
 * @returns {string} 完整的 npm 组织作用域包名
 */
export function getSubpackageName(key) {
  return `${SCOPE_PREFIX}${key}`;
}

/**
 * 解析并定位目标原生二进制可执行文件的绝对物理路径。
 *
 * @param {object} [options] 自定义解析选项 (便于单测注入)
 * @param {object} [options.env=process.env] 环境变量字典
 * @param {string} [options.platform=process.platform] 目标操作系统平台
 * @param {string} [options.arch=process.arch] 目标硬件架构
 * @param {Function} [options.subpackageResolver] 子包路径解析回调
 * @param {Function} [options.fileExists] 文件存在性校验函数
 * @returns {string} 目标原生二进制可执行文件的绝对路径
 * @throws {Error} 当在所有候选路径中均未能找到可执行文件时抛出排查异常
 */
export function resolveBinaryPath(options = {}) {
  const env = options.env || process.env;
  const platform = options.platform || process.platform;
  const arch = options.arch || process.arch;
  const subpackageResolver = options.subpackageResolver || defaultSubpackageResolver;
  const fileExists = options.fileExists || fs.existsSync;

  // 1. 优先支持环境变量显式覆盖指定二进制文件路径 (便于本地调试与容器环境挂载)
  if (env.MCP_SERVER_KUBERNETES_BINARY_PATH) {
    const customPath = path.resolve(env.MCP_SERVER_KUBERNETES_BINARY_PATH);
    if (fileExists(customPath)) {
      return customPath;
    }
    throw new Error(
      `环境变量 MCP_SERVER_KUBERNETES_BINARY_PATH 指定的可执行文件不存在: ${customPath}`
    );
  }

  // 2. 校验当前平台与架构是否受官方矩阵支持
  const matrixKey = getPlatformArchKey(platform, arch);
  const binaryFileName = platform === 'win32' ? `${BINARY_BASENAME}.exe` : BINARY_BASENAME;
  const subpackageName = getSubpackageName(matrixKey);

  // 3. 尝试从已安装的 optionalDependencies 对应平台子包中定位二进制
  try {
    const pkgJsonPath = subpackageResolver(subpackageName);
    const candidatePath = path.join(path.dirname(pkgJsonPath), 'bin', binaryFileName);
    if (fileExists(candidatePath)) {
      return candidatePath;
    }
  } catch {
    // 忽略找不到子包模块的异常，继续降级尝试本地开发路径
  }

  // 4. 开发构建阶段降级回退 (优先检索项目根目录下的 dist 或 output 产物)
  const devCandidatePath = path.resolve(
    __dirname,
    '../../dist',
    `${BINARY_BASENAME}-${platform}-${arch}`,
    binaryFileName
  );
  if (fileExists(devCandidatePath)) {
    return devCandidatePath;
  }

  // 5. 若全部检索路径皆未能命中，抛出友好的中文排查指引
  throw new Error(
    `未找到适用于当前平台 (${matrixKey}) 的原生可执行文件。\n` +
      `请检查是否成功安装了平台专有子包: ${subpackageName}\n` +
      `常见原因与解决办法:\n` +
      `1. 安装时使用了 --no-optional 参数，导致跳过了 optionalDependencies 的安装；\n` +
      `2. 您所在的网络环境无法从 npm 源拉取该平台的子包；\n` +
      `3. 您可通过设置环境变量 MCP_SERVER_KUBERNETES_BINARY_PATH 显式指定二进制文件位置。`
  );
}

/**
 * 默认使用 Node.js require.resolve 查找子包的 package.json 文件。
 *
 * @param {string} subpackageName 子包名称
 * @returns {string} 子包 package.json 路径
 */
function defaultSubpackageResolver(subpackageName) {
  return require.resolve(`${subpackageName}/package.json`);
}
