<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-brand">
        <div class="login-logo">💬</div>
        <div class="login-title">云客服工作台</div>
        <div class="login-sub">多渠道在线客服 · 企业微信风格</div>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent>
        <el-form-item label="账号" prop="account">
          <el-input
            v-model="form.account"
            size="large"
            placeholder="请输入账号"
            autocomplete="username"
            :prefix-icon="User"
            @keyup.enter="submit"
          />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            size="large"
            type="password"
            show-password
            placeholder="请输入密码"
            autocomplete="current-password"
            :prefix-icon="Lock"
            @keyup.enter="submit"
          />
        </el-form-item>

        <el-button
          class="login-submit"
          type="primary"
          size="large"
          :loading="auth.loading"
          @click="submit"
        >
          登 录
        </el-button>
      </el-form>

      <div class="login-tip">
        <div class="login-tip-title">演示账号</div>
        <div class="login-tip-row">
          <span><b>admin</b> / admin123</span>
          <el-tag size="small" type="success" effect="plain">管理员</el-tag>
        </div>
        <div class="login-tip-row">
          <span><b>kf01</b> / 123456</span>
          <el-tag size="small" effect="plain">客服小李</el-tag>
        </div>
        <div class="login-tip-row">
          <span><b>kf02</b> / 123456</span>
          <el-tag size="small" effect="plain">客服小王</el-tag>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { Lock, User } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const formRef = ref<FormInstance>()
const form = reactive({ account: 'admin', password: 'admin123' })

const rules: FormRules<typeof form> = {
  account: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

/** 当前所在端：'chat' | 'admin'（由 pathname 判断） */
function currentEnd(): 'chat' | 'admin' {
  return window.location.pathname.startsWith('/admin') ? 'admin' : 'chat'
}

/**
 * 跳到目标端的首页。三个端是各自独立的页面（admin.html / chat.html），
 * 跨端跳转必须整页刷新，仅靠 hash 路由无法切到另一个入口。
 */
function gotoEnd(end: 'chat' | 'admin', hash = ''): void {
  if (currentEnd() === end) {
    void router.replace(hash || '/')
    return
  }
  window.location.href = `/${end}/${hash}`
}

async function submit(): Promise<void> {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  try {
    await auth.login({ account: form.account, password: form.password })
    ElMessage.success(`欢迎回来，${auth.userName}`)

    const end = currentEnd()

    // 管理员端只允许管理员；客服（子账号）没有后台权限 → 直接带他去客服工作台
    if (end === 'admin' && !auth.isAdmin) {
      ElMessage.info('子账号无管理后台权限，已为你打开客服工作台')
      gotoEnd('chat')
      return
    }

    // 带了 redirect 且同端 → 优先跳回原页面
    const raw = route.query.redirect
    if (typeof raw === 'string' && raw.startsWith('/') && !raw.includes('/login')) {
      const targetEnd = raw.startsWith('/admin') ? 'admin' : 'chat'
      if (targetEnd === end) {
        await router.replace(raw)
        return
      }
      gotoEnd(targetEnd)
      return
    }

    gotoEnd(end)
  } catch {
    // 错误已由 http 拦截器提示
  }
}
</script>

<style scoped lang="scss">
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  background: linear-gradient(135deg, #f5f7fa 0%, #e9eaec 100%);
}

.login-card {
  width: 380px;
  padding: 32px 28px 24px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
}

.login-brand {
  text-align: center;
  margin-bottom: 24px;
}

.login-logo {
  width: 48px;
  height: 48px;
  margin: 0 auto 12px;
  border-radius: 12px;
  background: var(--kf-primary);
  color: #fff;
  font-size: 26px;
  line-height: 48px;
}

.login-title {
  font-size: 20px;
  font-weight: 600;
  color: var(--kf-text);
}

.login-sub {
  margin-top: 4px;
  font-size: 12px;
  color: var(--kf-text-sub);
}

.login-submit {
  width: 100%;
  margin-top: 4px;
  background: var(--kf-primary);
  border-color: var(--kf-primary);

  &:hover,
  &:focus {
    background: var(--kf-primary-dark);
    border-color: var(--kf-primary-dark);
  }
}

.login-tip {
  margin-top: 20px;
  padding: 12px 14px;
  background: var(--kf-bg);
  border-radius: var(--kf-radius);
  font-size: 12px;
  color: var(--kf-text-sub);
}

.login-tip-title {
  margin-bottom: 8px;
  font-weight: 600;
  color: var(--kf-text);
}

.login-tip-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 3px 0;

  b {
    color: var(--kf-text);
    font-family: ui-monospace, Consolas, monospace;
  }
}
</style>
