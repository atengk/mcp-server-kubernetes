#!/usr/bin/env node

/**
 * 跨平台子包构建与动态组装 CLI 脚本。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import path from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  buildSubpackages,
  syncMainPackageVersion,
} from '../lib/builder.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// 解析命令行参数
const args = process.argv.slice(2);
let version = process.env.VERSION || '0.0.0-development';
let outDir = path.resolve(__dirname, '../packages');
let binariesDir = path.resolve(__dirname, '../../dist');
let syncMain = false;

for (let i = 0; i < args.length; i++) {
  const arg = args[i];
  if (arg === '--version' && args[i + 1]) {
    version = args[++i];
  } else if (arg === '--out-dir' && args[i + 1]) {
    outDir = path.resolve(args[++i]);
  } else if (arg === '--bin-dir' && args[i + 1]) {
    binariesDir = path.resolve(args[++i]);
  } else if (arg === '--sync-main') {
    syncMain = true;
  }
}

try {
  console.log(`[构建] 开始为版本 ${version} 组装平台子包...`);
  console.log(`[构建] 目标输出路径: ${outDir}`);
  console.log(`[构建] 原生二进制来源: ${binariesDir}`);

  // 1. 生成并组装 5 大平台架构子包
  const created = buildSubpackages({
    outDir,
    version,
    binariesDir,
  });

  console.log(`[构建] 成功生成 ${created.length} 个平台子包:`);
  for (const dir of created) {
    console.log(`  - ${path.basename(dir)}`);
  }

  // 2. 若配置了同步主包，更新 npm/package.json
  if (syncMain) {
    const mainPkgPath = path.resolve(__dirname, '../package.json');
    syncMainPackageVersion(mainPkgPath, version);
    console.log(`[构建] 成功同步主包版本与 optionalDependencies: ${version}`);
  }

  console.log('[构建] 全部子包构建与组装工作已完成。');
} catch (err) {
  console.error(`[构建失败] ${err.message}`);
  process.exit(1);
}
