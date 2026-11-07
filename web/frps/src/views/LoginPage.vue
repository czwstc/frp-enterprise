<template>
  <div class="login-page">
    <div class="login-card">
      <div class="brand">
        <BrandMark />
      </div>

      <el-alert
        v-if="ssoNotice"
        :type="ssoNotice.type"
        :title="ssoNotice.title"
        :description="ssoNotice.desc"
        :closable="false"
        show-icon
        class="notice"
      />

      <!-- Recovery codes shown once after setting up the second factor -->
      <template v-if="step === 'recovery'">
        <h2>保存恢复密钥</h2>
        <p class="hint">
          二次验证已开启。下面的恢复密钥每个只能用一次，可以在手机丢失时代替动态验证码登录。它们只显示这一次，请现在妥善保存。
        </p>
        <div class="codes">
          <code v-for="c in recoveryCodes" :key="c">{{ c }}</code>
        </div>
        <div class="actions">
          <el-button @click="copy(recoveryCodes.join('\n'))"
            >复制全部</el-button
          >
          <el-button type="primary" @click="enter"
            >我已保存，进入控制台</el-button
          >
        </div>
      </template>

      <!-- First login: set up TOTP -->
      <template v-else-if="step === 'enroll'">
        <h2>设置二次验证</h2>
        <p class="hint">
          部署后的首次登录需要绑定动态验证码。用 Google Authenticator、Microsoft
          Authenticator、1Password 等应用扫描二维码，然后输入应用显示的 6
          位数字。
        </p>
        <div class="qr-wrap">
          <img
            v-if="qrDataURL"
            :src="qrDataURL"
            alt="TOTP QR code"
            class="qr"
          />
        </div>
        <div class="secret">
          无法扫码时手动输入密钥：
          <code>{{ formattedSecret }}</code>
          <el-button link type="primary" @click="copy(enroll.secret || '')"
            >复制</el-button
          >
        </div>
        <el-form @submit.prevent="onEnroll">
          <el-form-item>
            <el-input
              v-model="code"
              placeholder="6 位动态验证码"
              maxlength="6"
              inputmode="numeric"
              autocomplete="one-time-code"
              autofocus
            />
          </el-form-item>
          <el-button
            type="primary"
            native-type="submit"
            :loading="busy"
            class="full"
          >
            验证并登录
          </el-button>
        </el-form>
      </template>

      <!-- Password, then TOTP or a recovery code -->
      <template v-else>
        <el-form @submit.prevent="onLogin">
          <template v-if="step === 'password'">
            <el-form-item>
              <el-input
                v-model="username"
                placeholder="本地账号"
                autocomplete="username"
              />
            </el-form-item>
            <el-form-item>
              <el-input
                v-model="password"
                type="password"
                placeholder="密码"
                show-password
                autocomplete="current-password"
              />
            </el-form-item>
          </template>
          <template v-else>
            <p class="hint">
              {{
                useRecovery
                  ? '输入一个未使用过的恢复密钥。'
                  : '输入身份验证应用中的 6 位动态验证码。'
              }}
            </p>
            <el-form-item>
              <el-input
                v-model="code"
                :placeholder="
                  useRecovery ? '恢复密钥，例如 abcde-fghjk' : '6 位动态验证码'
                "
                :maxlength="useRecovery ? 11 : 6"
                autocomplete="one-time-code"
                autofocus
              />
            </el-form-item>
            <el-button
              link
              type="primary"
              class="switch"
              @click="toggleRecovery"
            >
              {{
                useRecovery ? '改用动态验证码' : '无法使用验证码？使用恢复密钥'
              }}
            </el-button>
          </template>
          <el-button
            type="primary"
            native-type="submit"
            :loading="busy"
            class="full"
          >
            {{ step === 'password' ? '登录' : '验证' }}
          </el-button>
          <el-button
            v-if="step === 'totp'"
            link
            class="full back"
            @click="reset"
            >返回</el-button
          >
        </el-form>

        <template v-if="config?.oidc && step === 'password'">
          <el-divider>或</el-divider>
          <el-button class="full" @click="ssoLogin"
            >使用 {{ config.oidcName }} 登录</el-button
          >
          <p class="hint small">
            首次通过 {{ config.oidcName }} 登录的账号需要由其他管理员批准。
          </p>
        </template>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import QRCode from 'qrcode'
import BrandMark from '../components/pulse/BrandMark.vue'
import { confirmEnroll, login } from '../api/auth'
import type { LoginResult } from '../api/auth'
import { useAuth } from '../composables/useAuth'

type Step = 'password' | 'totp' | 'enroll' | 'recovery'

const route = useRoute()
const router = useRouter()
const { config, loadConfig, refresh } = useAuth()

const step = ref<Step>('password')
const username = ref('')
const password = ref('')
const code = ref('')
const useRecovery = ref(false)
const busy = ref(false)
const enroll = ref<LoginResult>({ status: 'enroll' })
const qrDataURL = ref('')
const recoveryCodes = ref<string[]>([])

const formattedSecret = computed(() =>
  (enroll.value.secret || '').replace(/(.{4})/g, '$1 ').trim(),
)

const ssoNotice = computed(() => {
  const status = route.query.sso as string | undefined
  const msg = (route.query.msg as string | undefined) || ''
  switch (status) {
    case 'pending':
      return {
        type: 'warning' as const,
        title: '等待管理员批准',
        desc: `账号 ${msg} 尚未登记，已提交登录申请。请联系其他管理员在“用户”页批准后重新登录。`,
      }
    case 'rejected':
      return {
        type: 'error' as const,
        title: '登录申请已被拒绝',
        desc: `账号 ${msg} 的登录申请已被管理员拒绝。`,
      }
    case 'disabled':
      return {
        type: 'error' as const,
        title: '账号已停用',
        desc: `账号 ${msg} 已被管理员停用。`,
      }
    case 'error':
      return { type: 'error' as const, title: 'SSO 登录失败', desc: msg }
    default:
      return null
  }
})

watch(
  () => enroll.value.otpauthURL,
  async (url) => {
    qrDataURL.value = url
      ? await QRCode.toDataURL(url, { margin: 1, width: 200 })
      : ''
  },
)

const copy = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {
    ElMessage.info(text)
  }
}

const toggleRecovery = () => {
  useRecovery.value = !useRecovery.value
  code.value = ''
}

const reset = () => {
  step.value = 'password'
  code.value = ''
  useRecovery.value = false
}

const enter = async () => {
  await refresh()
  router.replace('/')
}

const onLogin = async () => {
  if (!username.value || !password.value) {
    ElMessage.warning('请输入账号和密码')
    return
  }
  busy.value = true
  try {
    const res = await login(
      username.value,
      password.value,
      step.value === 'totp' ? code.value : '',
    )
    switch (res.status) {
      case 'enroll':
        enroll.value = res
        code.value = ''
        step.value = 'enroll'
        break
      case 'totp_required':
        code.value = ''
        step.value = 'totp'
        break
      default:
        if (res.recoveryUsed) {
          ElMessage.warning(`已使用一个恢复密钥，还剩 ${res.recoveryLeft} 个`)
        }
        await enter()
    }
  } catch (e: any) {
    ElMessage.error(e.message)
    if (step.value === 'totp') code.value = ''
  } finally {
    busy.value = false
  }
}

const onEnroll = async () => {
  busy.value = true
  try {
    const res = await confirmEnroll(enroll.value.enrollToken || '', code.value)
    recoveryCodes.value = res.recoveryCodes || []
    password.value = ''
    step.value = 'recovery'
  } catch (e: any) {
    ElMessage.error(e.message)
    code.value = ''
  } finally {
    busy.value = false
  }
}

const ssoLogin = () => {
  window.location.href = '../api/auth/oidc/login'
}

onMounted(async () => {
  const cfg = await loadConfig()
  if (!cfg.enabled) {
    router.replace('/')
    return
  }
  if (!route.query.sso && (await refresh())) {
    router.replace('/')
  }
})
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: var(--pulse-canvas);
}

.login-card {
  width: 100%;
  max-width: 400px;
  padding: 32px;
  border-radius: 12px;
  background: var(--pulse-card);
  border: 1px solid var(--pulse-line);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.06);
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.logo {
  width: 40px;
  height: 40px;
}

.title {
  font-size: 20px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.subtitle {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

h2 {
  font-size: 18px;
  margin: 0 0 8px;
  color: var(--el-text-color-primary);
}

.hint {
  font-size: 13px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
  margin: 0 0 16px;
}

.hint.small {
  margin-top: 12px;
  font-size: 12px;
}

.notice {
  margin-bottom: 20px;
}

.full {
  width: 100%;
}

.back {
  margin: 8px 0 0;
}

.switch {
  margin: -8px 0 12px;
  padding: 0;
}

.qr-wrap {
  display: flex;
  justify-content: center;
  margin: 8px 0 12px;
}

.qr {
  width: 200px;
  height: 200px;
  border-radius: 8px;
  background: #fff;
}

.secret {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 16px;
  word-break: break-all;
}

.secret code,
.codes code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--el-text-color-primary);
}

.codes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 12px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  margin-bottom: 16px;
}

.codes code {
  font-size: 14px;
  text-align: center;
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
