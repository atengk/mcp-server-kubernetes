/**
 * 提供跨平台子包构建、元数据生成与版本同步核心逻辑。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import fs from 'node:fs';
import path from 'node:path';
import {
  BINARY_BASENAME,
  SCOPE_PREFIX,
} from './resolver.js';

/**
 * 平台与芯片架构目标元数据定义矩阵。
 */
export const PACKAGE_MATRIX = [
  { target: 'darwin-arm64', os: 'darwin', cpu: 'arm64' },
  { target: 'darwin-x64', os: 'darwin', cpu: 'x64' },
  { target: 'linux-x64', os: 'linux', cpu: 'x64' },
  { target: 'linux-arm64', os: 'linux', cpu: 'arm64' },
  { target: 'win32-x64', os: 'win32', cpu: 'x64' },
].map((item) => ({
  ...item,
  subpackage: `${SCOPE_PREFIX}${item.target}`,
  binaryName: item.os === 'win32' ? `${BINARY_BASENAME}.exe` : BINARY_BASENAME,
}));

/**
 * 为指定平台目标生成标准子包 package.json 属性定义。
 *
 * @param {object} targetMeta 平台目标元数据
 * @param {string} version 注入的语义化版本号
 * @returns {object} 子包 package.json 结构体对象
 */
export function generateSubpackageJson(targetMeta, version) {
  const currentVersion = version || '0.0.0-development';
  return {
    name: targetMeta.subpackage,
    version: currentVersion,
    description: `Native binary of mcp-server-kubernetes for ${targetMeta.target}`,
    homepage: 'https://github.com/atengk/mcp-server-kubernetes',
    repository: {
      type: 'git',
      url: 'https://github.com/atengk/mcp-server-kubernetes.git',
    },
    license: 'Apache-2.0',
    author: 'Ateng',
    os: [targetMeta.os],
    cpu: [targetMeta.cpu],
    files: ['bin'],
  };
}

/**
 * 执行 5 大平台子包目录生成与动态二进制组装。
 *
 * @param {object} options 组装选项
 * @param {string} options.outDir 输出根目录
 * @param {string} [options.version='0.0.0-development'] 发布的版本号
 * @param {string} [options.binariesDir] 原生二进制来源目录（可选，若提供则将对应二进制拷贝至各子包 bin/ 下）
 * @returns {Array<string>} 生成的子包目录绝对路径列表
 */
export function buildSubpackages(options = {}) {
  const outDir = options.outDir;
  if (!outDir) {
    throw new Error('必须指定子包输出目录 outDir');
  }

  const version = options.version || '0.0.0-development';
  const binariesDir = options.binariesDir;
  const createdDirs = [];

  // 1. 确保输出根目录存在
  fs.mkdirSync(outDir, { recursive: true });

  // 2. 遍历平台矩阵逐一生成各子包目录与 package.json
  for (const meta of PACKAGE_MATRIX) {
    const pkgDirName = meta.target;
    const subpackageDir = path.join(outDir, pkgDirName);
    const binDir = path.join(subpackageDir, 'bin');

    fs.mkdirSync(binDir, { recursive: true });

    // 写入专属 package.json
    const pkgJsonContent = generateSubpackageJson(meta, version);
    const pkgJsonPath = path.join(subpackageDir, 'package.json');
    fs.writeFileSync(
      pkgJsonPath,
      JSON.stringify(pkgJsonContent, null, 2) + '\n',
      'utf8'
    );

    // 3. 若提供了二进制来源目录，定位并拷贝对应平台的二进制文件
    if (binariesDir) {
      const sourceBinary = path.join(
        binariesDir,
        `mcp-server-kubernetes-${meta.target}`,
        meta.binaryName
      );

      if (fs.existsSync(sourceBinary)) {
        const destBinaryPath = path.join(binDir, meta.binaryName);
        fs.copyFileSync(sourceBinary, destBinaryPath);
        // 在非 Windows 平台赋予执行权限
        if (process.platform !== 'win32') {
          try {
            fs.chmodSync(destBinaryPath, 0o755);
          } catch (err) {
            console.warn(`[构建警告] 设置执行权限失败 (${destBinaryPath}): ${err.message}`);
          }
        }
      }
    }

    createdDirs.push(subpackageDir);
  }

  return createdDirs;
}

/**
 * 同步主包 package.json 中的自身版本及 optionalDependencies 子包版本。
 *
 * @param {string} pkgJsonPath 主包 package.json 文件绝对路径
 * @param {string} newVersion 新的语义化版本号
 * @returns {object} 更新后的主包 JSON 结构体对象
 */
export function syncMainPackageVersion(pkgJsonPath, newVersion) {
  if (!fs.existsSync(pkgJsonPath)) {
    throw new Error(`主包配置文件不存在: ${pkgJsonPath}`);
  }

  const raw = fs.readFileSync(pkgJsonPath, 'utf8');
  const pkg = JSON.parse(raw);

  pkg.version = newVersion;
  if (pkg.optionalDependencies && typeof pkg.optionalDependencies === 'object') {
    for (const key of Object.keys(pkg.optionalDependencies)) {
      pkg.optionalDependencies[key] = newVersion;
    }
  }

  fs.writeFileSync(pkgJsonPath, JSON.stringify(pkg, null, 2) + '\n', 'utf8');
  return pkg;
}
