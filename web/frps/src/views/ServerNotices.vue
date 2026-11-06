<template>
  <div class="ent-page">
    <div class="title-section">
      <h1 class="page-title">通知</h1>
      <p class="page-subtitle">
        向在线客户端发送通知，客户端会在“通知”页和弹窗中看到。离线客户端在下次连接时收到最近的通知。
      </p>
    </div>

    <div class="grid">
      <el-card shadow="never" class="panel">
        <template #header>发送通知</template>
        <el-form label-position="top" @submit.prevent>
          <el-form-item label="级别">
            <el-radio-group v-model="form.level">
              <el-radio-button value="info">通知</el-radio-button>
              <el-radio-button value="success">完成</el-radio-button>
              <el-radio-button value="warning">警告</el-radio-button>
              <el-radio-button value="error">紧急</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="标题">
            <el-input v-model="form.title" maxlength="80" show-word-limit />
          </el-form-item>
          <el-form-item label="内容">
            <el-input
              v-model="form.content"
              type="textarea"
              :rows="4"
              maxlength="2000"
            />
          </el-form-item>
          <el-form-item label="发送给">
            <el-select v-model="form.target" filterable class="target">
              <el-option label="全部客户端" value="" />
              <el-option
                v-for="c in approvedCodes"
                :key="c.code"
                :label="`${c.code}${c.hostname ? ' · ' + c.hostname : ''}${c.note ? ' · ' + c.note : ''}`"
                :value="c.code"
              />
            </el-select>
          </el-form-item>
          <el-button type="primary" :loading="sending" @click="onSend"
            >发送</el-button
          >
        </el-form>
      </el-card>

      <el-card shadow="never" class="panel">
        <template #header>服务端端口占用</template>
        <el-table :data="ports" empty-text="暂无已转发的端口" size="small">
          <el-table-column prop="proto" label="协议" width="70" />
          <el-table-column prop="port" label="端口" width="80" />
          <el-table-column prop="proxyName" label="代理" min-width="140" />
          <el-table-column label="鉴权码" min-width="170">
            <template #default="{ row }">{{ row.authCode || '-' }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </div>

    <el-card shadow="never" class="panel">
      <template #header>历史通知</template>
      <el-empty v-if="notices.length === 0" description="还没有发送过通知" />
      <div v-for="n in notices" :key="n.id" class="notice">
        <div class="notice-head">
          <el-tag :type="levelTag(n.level)" size="small">{{
            levelLabel(n.level)
          }}</el-tag>
          <strong>{{ n.title }}</strong>
          <span class="muted">{{ new Date(n.time).toLocaleString() }}</span>
          <span class="muted">→ {{ n.target || '全部客户端' }}</span>
        </div>
        <div v-if="n.content" class="notice-content">{{ n.content }}</div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  listAuthCodes,
  listNotices,
  listPortsInUse,
  sendNotice,
} from '../api/enterprise'
import type { AuthCode, Notice, PortInUse } from '../types/enterprise'

const form = reactive({ level: 'info', title: '', content: '', target: '' })
const sending = ref(false)
const notices = ref<Notice[]>([])
const codes = ref<AuthCode[]>([])
const ports = ref<PortInUse[]>([])
let timer: number | null = null

const approvedCodes = computed(() =>
  codes.value.filter((c) => c.status === 'approved'),
)

const levelLabel = (l: string) =>
  ({ info: '通知', success: '完成', warning: '警告', error: '紧急' })[l] || l
const levelTag = (l: string) =>
  (
    ({
      info: 'primary',
      success: 'success',
      warning: 'warning',
      error: 'danger',
    }) as Record<string, 'primary' | 'success' | 'warning' | 'danger'>
  )[l] || 'primary'

const fetchData = async () => {
  try {
    const [n, c, p] = await Promise.all([
      listNotices(),
      listAuthCodes(),
      listPortsInUse(),
    ])
    notices.value = n
    codes.value = c
    ports.value = p
  } catch (e: any) {
    ElMessage.error('加载失败：' + e.message)
    if (timer !== null) {
      window.clearInterval(timer)
      timer = null
    }
  }
}

const onSend = async () => {
  if (!form.title.trim() && !form.content.trim()) {
    ElMessage.warning('请填写标题或内容')
    return
  }
  sending.value = true
  try {
    await sendNotice({ ...form })
    ElMessage.success('已发送')
    form.title = ''
    form.content = ''
    await fetchData()
  } catch (e: any) {
    ElMessage.error('发送失败：' + e.message)
  } finally {
    sending.value = false
  }
}

onMounted(() => {
  fetchData()
  timer = window.setInterval(fetchData, 5000)
})

onUnmounted(() => {
  if (timer !== null) window.clearInterval(timer)
})
</script>

<style scoped>
.ent-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.title-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.page-title {
  font-size: 28px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
  line-height: 1.2;
}

.page-subtitle {
  font-size: 14px;
  color: var(--el-text-color-secondary);
  margin: 0;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 20px;
}

.panel {
  border-radius: 8px;
}

.target {
  width: 100%;
}

.notice {
  padding: 12px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.notice:last-child {
  border-bottom: none;
}

.notice-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.notice-content {
  margin-top: 6px;
  white-space: pre-wrap;
  color: var(--el-text-color-regular);
}

.muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
