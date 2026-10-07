/**
 * 针对子包生成器与主包版本同步逻辑开展单元测试。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import {
  PACKAGE_MATRIX,
  generateSubpackageJson,
  buildSubpackages,
  syncMainPackageVersion,
} from '../lib/builder.js';

describe('子包元数据与构建器 (Builder)', () => {
  let tempDir;

  beforeEach(() => {
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mcp-builder-test-'));
  });

  afterEach(() => {
    if (tempDir && fs.existsSync(tempDir)) {
      fs.rmSync(tempDir, { recursive: true, force: true });
    }
  });

  test('平台目标元数据矩阵包含完整 5 大平台', () => {
    assert.equal(PACKAGE_MATRIX.length, 5);
    const targets = PACKAGE_MATRIX.map((m) => m.target);
    assert.deepEqual(targets, [
      'darwin-arm64',
      'darwin-x64',
      'linux-x64',
      'linux-arm64',
      'win32-x64',
    ]);
  });

  test('generateSubpackageJson 正确配置平台专有 os 与 cpu 属性', () => {
    const darwinMeta = PACKAGE_MATRIX.find((m) => m.target === 'darwin-arm64');
    const pkgJson = generateSubpackageJson(darwinMeta, '1.2.3');

    assert.equal(pkgJson.name, '@atengk/mcp-server-kubernetes-darwin-arm64');
    assert.equal(pkgJson.version, '1.2.3');
    assert.deepEqual(pkgJson.os, ['darwin']);
    assert.deepEqual(pkgJson.cpu, ['arm64']);
    assert.deepEqual(pkgJson.files, ['bin']);
  });

  test('buildSubpackages 正确创建 5 大子包目录与 package.json', () => {
    const outDir = path.join(tempDir, 'packages');
    const created = buildSubpackages({
      outDir,
      version: '2.0.0',
    });

    assert.equal(created.length, 5);

    for (const meta of PACKAGE_MATRIX) {
      const subDir = path.join(outDir, meta.target);
      assert.ok(fs.existsSync(subDir), `子包目录应存在: ${meta.target}`);

      const pkgJsonPath = path.join(subDir, 'package.json');
      assert.ok(fs.existsSync(pkgJsonPath), `package.json 应存在: ${pkgJsonPath}`);

      const content = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8'));
      assert.equal(content.name, meta.subpackage);
      assert.equal(content.version, '2.0.0');
      assert.deepEqual(content.os, [meta.os]);
      assert.deepEqual(content.cpu, [meta.cpu]);

      const binDir = path.join(subDir, 'bin');
      assert.ok(fs.existsSync(binDir), 'bin 目录应存在');
    }
  });

  test('buildSubpackages 能够正确发现并复制二进制文件到 bin 目录', () => {
    const outDir = path.join(tempDir, 'packages');
    const binariesDir = path.join(tempDir, 'dist');
    fs.mkdirSync(binariesDir, { recursive: true });

    // 模拟构建出的二进制产物
    for (const meta of PACKAGE_MATRIX) {
      const targetDist = path.join(binariesDir, `mcp-server-kubernetes-${meta.target}`);
      fs.mkdirSync(targetDist, { recursive: true });
      fs.writeFileSync(path.join(targetDist, meta.binaryName), 'mock-binary-content');
    }

    buildSubpackages({
      outDir,
      version: '1.0.0',
      binariesDir,
    });

    for (const meta of PACKAGE_MATRIX) {
      const binaryPath = path.join(outDir, meta.target, 'bin', meta.binaryName);
      assert.ok(fs.existsSync(binaryPath), `二进制文件应被成功复制: ${binaryPath}`);
      const content = fs.readFileSync(binaryPath, 'utf8');
      assert.equal(content, 'mock-binary-content');
    }
  });

  test('syncMainPackageVersion 正确更新主包版本与所有 optionalDependencies', () => {
    const fakeMainPkgPath = path.join(tempDir, 'package.json');
    const initialConfig = {
      name: '@atengk/mcp-server-kubernetes',
      version: '0.0.0-development',
      optionalDependencies: {
        '@atengk/mcp-server-kubernetes-darwin-arm64': '0.0.0-development',
        '@atengk/mcp-server-kubernetes-linux-x64': '0.0.0-development',
      },
    };
    fs.writeFileSync(fakeMainPkgPath, JSON.stringify(initialConfig, null, 2), 'utf8');

    const updated = syncMainPackageVersion(fakeMainPkgPath, '3.1.4');
    assert.equal(updated.version, '3.1.4');
    assert.equal(
      updated.optionalDependencies['@atengk/mcp-server-kubernetes-darwin-arm64'],
      '3.1.4'
    );
    assert.equal(
      updated.optionalDependencies['@atengk/mcp-server-kubernetes-linux-x64'],
      '3.1.4'
    );

    const reloaded = JSON.parse(fs.readFileSync(fakeMainPkgPath, 'utf8'));
    assert.equal(reloaded.version, '3.1.4');
    assert.equal(
      reloaded.optionalDependencies['@atengk/mcp-server-kubernetes-linux-x64'],
      '3.1.4'
    );
  });
});
