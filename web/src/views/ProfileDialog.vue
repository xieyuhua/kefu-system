<script setup lang="ts">
/**
 * 个人资料弹窗：修改昵称、头像、密码。
 * 从客服工作台侧边栏头像或后台右上角用户名进入。
 */
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Lock, Upload, User } from '@element-plus/icons-vue'
import { changePassword, updateProfile, type ProfileResult } from '@/api/profile'
import { uploadAttachment, validateFile } from '@/api/upload'
import { useAuthStore } from '@/stores/auth'
import ThumbImage from '@/components/ThumbImage.vue'

const visible = defineModel<boolean>({ default: false })

const auth = useAuthStore()

type Tab = 'profile' | 'password'
const tab = ref<Tab>('profile')

// ---------- 资料 ----------
const name = ref('')
const avatar = ref('')
const savingProfile = ref(false)
const fileRef = ref<HTMLInputElement | null>(null)
const uploading = ref(false)

// ---------- 密码 ----------
const oldPwd = ref('')
const newPwd = ref('')
const confirmPwd = ref('')
const savingPwd = ref(false)

const account = computed(() => auth.user?.account || '')
const roleLabel = computed(() => (auth.isAdmin ? '管理员' : '客服'))

/** 打开时用当前登录用户信息填充表单 */
watch(visible, (open) => {
  if (!open) return
  tab.value = 'profile'
  name.value = auth.user?.name || ''
  avatar.value = auth.user?.avatar || ''
  oldPwd.value = ''
  newPwd.value = ''
  confirmPwd.value = ''
})

/** 选择新头像并上传 */
function pickAvatar(): void {
  fileRef.value?.click()
}

async function onAvatarFile(): Promise<void> {
  const el = fileRef.value
  const file = el?.files?.[0]
  if (!el || !file) return

  if (!/\.(jpg|jpeg|png|gif|webp|bmp)$/i.test(file.name)) {
    ElMessage.error('头像仅支持图片格式')
    el.value = ''
    return
  }
  const err = validateFile(file)
  if (err) {
    ElMessage.error(err)
    el.value = ''
    return
  }

  uploading.value = true
  try {
    const payload = await uploadAttachment(file)
    avatar.value = payload.url
    ElMessage.success('头像已上传，记得点保存')
  } catch {
    ElMessage.error('头像上传失败')
  } finally {
    uploading.value = false
    el.value = ''
  }
}

/** 保存昵称与头像 */
async function saveProfile(): Promise<void> {
  const n = name.value.trim()
  if (!n) {
    ElMessage.warning('昵称不能为空')
    return
  }
  savingProfile.value = true
  try {
    const res: ProfileResult = await updateProfile({
      name: n,
      avatar: avatar.value.trim(),
    })
    // 同步到本地 store，界面各处昵称/头像立即更新
    if (auth.user) {
      auth.user = { ...auth.user, name: res.name, avatar: res.avatar }
    }
    ElMessage.success('资料已保存')
    visible.value = false
  } catch {
    // 错误已由拦截器提示
  } finally {
    savingProfile.value = false
  }
}

/** 保存新密码 */
async function savePassword(): Promise<void> {
  if (!oldPwd.value) {
    ElMessage.warning('请输入原密码')
    return
  }
  if (newPwd.value.length < 6) {
    ElMessage.warning('新密码至少 6 位')
    return
  }
  if (newPwd.value !== confirmPwd.value) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  savingPwd.value = true
  try {
    await changePassword({ old_password: oldPwd.value, new_password: newPwd.value })
    ElMessage.success('密码修改成功，请牢记新密码')
    oldPwd.value = ''
    newPwd.value = ''
    confirmPwd.value = ''
    tab.value = 'profile'
  } catch {
    // 错误已由拦截器提示
  } finally {
    savingPwd.value = false
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    title="个人资料"
    width="440px"
    :close-on-click-modal="false"
    append-to-body
  >
    <el-tabs v-model="tab">
      <!-- 基本资料 -->
      <el-tab-pane label="基本资料" name="profile">
        <div class="avatar-row">
          <ThumbImage :src="avatar" :size="72" round kind="avatar" :fallback="'我'" />
          <div class="avatar-row__actions">
            <el-button size="small" :icon="Upload" :loading="uploading" @click="pickAvatar">
              更换头像
            </el-button>
            <div class="avatar-row__hint">支持 jpg / png / webp，建议方形</div>
          </div>
        </div>

        <el-form label-width="72px" class="form">
          <el-form-item label="账号">
            <span class="readonly">{{ account }}</span>
            <el-tag size="small" type="info" effect="plain">{{ roleLabel }}</el-tag>
          </el-form-item>
          <el-form-item label="昵称">
            <el-input v-model="name" maxlength="20" show-word-limit placeholder="请输入昵称" />
          </el-form-item>
          <el-form-item label="头像地址">
            <el-input v-model="avatar" placeholder="可直接填写图片地址" />
          </el-form-item>
        </el-form>

        <input ref="fileRef" type="file" hidden accept="image/*" @change="onAvatarFile" />
      </el-tab-pane>

      <!-- 修改密码 -->
      <el-tab-pane label="修改密码" name="password">
        <el-form label-width="80px" class="form">
          <el-form-item label="原密码">
            <el-input v-model="oldPwd" type="password" show-password placeholder="请输入原密码" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input
              v-model="newPwd"
              type="password"
              show-password
              placeholder="至少 6 位"
            />
          </el-form-item>
          <el-form-item label="确认密码">
            <el-input
              v-model="confirmPwd"
              type="password"
              show-password
              placeholder="再次输入新密码"
            />
          </el-form-item>
        </el-form>
        <el-alert type="info" :closable="false" class="tip">
          修改密码后当前登录状态仍有效，下次登录请使用新密码。
        </el-alert>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button
        v-if="tab === 'profile'"
        type="primary"
        :loading="savingProfile"
        :icon="User"
        @click="saveProfile"
      >
        保存资料
      </el-button>
      <el-button
        v-else
        type="primary"
        :loading="savingPwd"
        :icon="Lock"
        @click="savePassword"
      >
        修改密码
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.avatar-row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 4px 0 16px;
  border-bottom: 1px solid #f0f0f0;
  margin-bottom: 14px;

  &__actions {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }

  &__hint {
    font-size: 12px;
    color: #7f7f7f;
  }
}

.form {
  :deep(.el-form-item) {
    margin-bottom: 16px;
  }
}

.readonly {
  color: #191919;
  margin-right: 8px;
}

.tip {
  margin-top: 4px;
}
</style>