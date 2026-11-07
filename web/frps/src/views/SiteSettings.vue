<template>
  <div class="prefs">
    <p class="intro">
      外观设置保存在服务端：所有管理员、登录页和已连接的桌面客户端都使用同一套名称、图标和强调色。
      <span v-if="saved.updatedBy"
        >上次由 {{ saved.updatedBy }} 于 {{ fmt(saved.updatedAt) }} 修改。</span
      >
    </p>

    <form class="form" @submit.prevent="onSave">
      <section class="section">
        <h3>品牌</h3>
        <p>左上角的图标和名称，也用于登录页、浏览器标签和桌面客户端。</p>
        <div class="field">
          <label for="pf-name">站点名称</label>
          <el-input
            id="pf-name"
            v-model="form.siteName"
            maxlength="40"
            show-word-limit
            class="w"
          />
        </div>
        <div class="field">
          <label for="pf-sub">副标题</label>
          <el-input
            id="pf-sub"
            v-model="form.subtitle"
            maxlength="60"
            class="w"
          />
        </div>
        <div class="field">
          <label>左上角图标</label>
          <div class="ctl">
            <span class="logo-box" :class="`bd-${form.logoBackdrop}`">
              <img :src="form.logo || defaultLogo" alt="" />
            </span>
            <label class="upload">
              上传图片
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp,image/svg+xml"
                @change="pick($event, 'logo')"
              />
            </label>
            <el-button v-if="form.logo" @click="form.logo = ''"
              >恢复默认</el-button
            >
            <span class="hint"
              >PNG、JPEG、WebP 或 SVG，不超过 512 KB；横版标志建议高度 60
              像素以上。</span
            >
          </div>
        </div>
        <div class="field">
          <label>图标底衬</label>
          <div class="ctl">
            <el-radio-group v-model="form.logoBackdrop">
              <el-radio-button value="halo">黑色光晕</el-radio-button>
              <el-radio-button value="plate">深色底板</el-radio-button>
              <el-radio-button value="none">无</el-radio-button>
            </el-radio-group>
            <span class="hint"
              >白色或浅色的透明图标在浅色背景上需要光晕或底板才看得清。</span
            >
          </div>
        </div>
        <div class="field">
          <label>图标显示</label>
          <el-radio-group v-model="form.logoMode">
            <el-radio-button value="both">图标 + 名称</el-radio-button>
            <el-radio-button value="logo">只显示图标</el-radio-button>
          </el-radio-group>
        </div>
      </section>

      <section class="section">
        <h3>主题</h3>
        <p>
          强调色用于选中的侧栏项、主要按钮和链接；卡片的指标颜色保持不变，便于区分。
        </p>
        <div class="field">
          <label>明暗</label>
          <el-radio-group v-model="form.theme">
            <el-radio-button value="system">跟随系统</el-radio-button>
            <el-radio-button value="light">浅色</el-radio-button>
            <el-radio-button value="dark">深色</el-radio-button>
          </el-radio-group>
        </div>
        <div class="field">
          <label>强调色</label>
          <div class="ctl">
            <button
              v-for="a in accents"
              :key="a.color"
              type="button"
              class="sw"
              :style="{ background: a.color }"
              :aria-pressed="form.accent === a.color"
              :title="a.name"
              :aria-label="a.name"
              @click="form.accent = a.color"
            ></button>
            <el-color-picker
              v-model="form.accent"
              :predefine="accents.map((a) => a.color)"
            />
            <code class="mono">{{ form.accent }}</code>
          </div>
        </div>
      </section>

      <section class="section">
        <h3>右上角链接</h3>
        <p>替换原来的 GitHub 图标，可以指向公司官网、内部 Wiki 或工单系统。</p>
        <div class="field">
          <label>显示</label>
          <el-radio-group v-model="form.topLink.mode">
            <el-radio-button value="custom">自定义链接</el-radio-button>
            <el-radio-button value="github">GitHub 图标</el-radio-button>
            <el-radio-button value="none">不显示</el-radio-button>
          </el-radio-group>
        </div>
        <template v-if="form.topLink.mode === 'custom'">
          <div class="field">
            <label for="pf-url">链接地址</label>
            <el-input
              id="pf-url"
              v-model="form.topLink.url"
              placeholder="https://"
              class="w"
            />
          </div>
          <div class="field">
            <label for="pf-text">提示文字</label>
            <el-input
              id="pf-text"
              v-model="form.topLink.text"
              maxlength="30"
              class="w"
            />
          </div>
          <div class="field">
            <label>图标</label>
            <div class="ctl">
              <span class="logo-box small">
                <img
                  v-if="form.topLink.image"
                  :src="form.topLink.image"
                  alt=""
                />
                <span v-else class="muted">无</span>
              </span>
              <label class="upload">
                上传图片
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  @change="pick($event, 'link')"
                />
              </label>
              <el-button
                v-if="form.topLink.image"
                @click="form.topLink.image = ''"
                >移除</el-button
              >
            </div>
          </div>
        </template>
        <div class="preview">
          <span class="muted">右上角预览</span>
          <span class="spacer"></span>
          <TopLink :link="form.topLink" />
        </div>
      </section>

      <div class="save-bar">
        <el-button @click="resetDefaults">恢复全部默认</el-button>
        <el-button @click="revert" :disabled="!dirty">放弃修改</el-button>
        <el-button
          type="primary"
          native-type="submit"
          :loading="saving"
          :disabled="!dirty"
          >保存设置</el-button
        >
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import TopLink from '../components/pulse/TopLink.vue'
import defaultLogo from '../assets/huiding-logo.png'
import { getBranding, saveBranding } from '../api/branding'
import type { Branding } from '../api/branding'
import { defaultBranding, useBranding } from '../composables/useBranding'

const { applyBranding } = useBranding()

const accents = [
  { name: '汇鼎蓝', color: '#1f5eff' },
  { name: '青', color: '#0e8f96' },
  { name: '绿', color: '#1f9d55' },
  { name: '橙', color: '#e2701b' },
  { name: '紫', color: '#6c4ee6' },
  { name: '石墨', color: '#3a4252' },
]

const clone = (b: Branding): Branding => JSON.parse(JSON.stringify(b))
const saved = ref<Branding>(defaultBranding())
const form = ref<Branding>(defaultBranding())
const saving = ref(false)

const dirty = computed(
  () => JSON.stringify(form.value) !== JSON.stringify(saved.value),
)

// Preview the look live while editing; leaving without saving restores it.
watch(
  form,
  (f) => {
    if (/^#[0-9a-fA-F]{6}$/.test(f.accent || '')) applyBranding(clone(f))
  },
  { deep: true },
)

const fmt = (t?: string) =>
  t && !t.startsWith('0001') ? new Date(t).toLocaleString() : ''

const pick = (e: Event, target: 'logo' | 'link') => {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  if (f.size > 512 * 1024) {
    ElMessage.error('图片不能超过 512 KB')
    return
  }
  const fr = new FileReader()
  fr.onload = () => {
    const url = String(fr.result)
    if (target === 'logo') form.value.logo = url
    else form.value.topLink.image = url
  }
  fr.readAsDataURL(f)
}

const load = async () => {
  try {
    saved.value = await getBranding()
    form.value = clone(saved.value)
  } catch (e: any) {
    ElMessage.error('读取设置失败：' + e.message)
  }
}

const onSave = async () => {
  saving.value = true
  try {
    form.value.accent = (form.value.accent || '').toLowerCase()
    saved.value = await saveBranding(form.value)
    form.value = clone(saved.value)
    applyBranding(clone(saved.value))
    ElMessage.success('已保存，已连接的客户端会同步新的名称、图标和强调色')
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    saving.value = false
  }
}

const revert = () => {
  form.value = clone(saved.value)
}

const resetDefaults = () => {
  const d = defaultBranding()
  d.version = saved.value.version
  form.value = d
}

onMounted(load)
onBeforeUnmount(() => applyBranding(clone(saved.value)))
</script>

<style scoped>
.prefs {
  display: grid;
  gap: 12px;
  max-width: 860px;
}
.intro {
  margin: 0;
  font-size: 12.5px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.form {
  display: grid;
  gap: 12px;
}
.section {
  background: var(--pulse-card);
  border: 1px solid var(--pulse-line);
  border-radius: 12px;
  padding: 14px 16px;
  display: grid;
  gap: 12px;
  box-shadow: var(--pulse-shadow);
}
.section h3 {
  margin: 0;
  font-size: 13.5px;
}
.section > p {
  margin: -6px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.field {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 12px;
  align-items: center;
}
.field > label {
  color: var(--el-text-color-secondary);
  font-size: 12.5px;
}
.ctl {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  min-width: 0;
}
.w {
  max-width: 360px;
}
.logo-box {
  width: 160px;
  height: 44px;
  border: 1px dashed var(--pulse-line);
  border-radius: 8px;
  display: grid;
  place-items: center;
  background: var(--pulse-side);
}
.logo-box.small {
  width: 44px;
}
.logo-box img {
  max-height: 26px;
  max-width: 140px;
}
.logo-box.bd-halo img {
  filter: drop-shadow(0 0 1px rgba(0, 0, 0, 0.85))
    drop-shadow(0 0 2px rgba(0, 0, 0, 0.6))
    drop-shadow(0 1px 4px rgba(0, 0, 0, 0.35));
}
.logo-box.bd-plate {
  background: #1b2233;
  border-style: solid;
}
.upload {
  position: relative;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  height: 32px;
  padding: 0 12px;
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  font-size: 13px;
  cursor: pointer;
  color: var(--el-text-color-regular);
  background: var(--el-bg-color);
}
.upload:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.upload input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}
.hint,
.muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.sw {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 2px solid var(--pulse-card);
  box-shadow: 0 0 0 1px var(--pulse-line);
  cursor: pointer;
  padding: 0;
}
.sw[aria-pressed='true'] {
  box-shadow: 0 0 0 2px var(--el-text-color-primary);
}
.mono {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.preview {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--pulse-side);
}
.spacer {
  flex: 1;
}
.save-bar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}
@media (max-width: 640px) {
  .field {
    grid-template-columns: minmax(0, 1fr);
    gap: 6px;
  }
}
</style>
