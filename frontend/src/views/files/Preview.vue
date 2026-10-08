<template>
  <div id="previewer">
    <div
      v-if="showTriageControls && !isDeleted"
      class="triage-preview-controls"
      role="toolbar"
      aria-label="Triage"
    >
      <button type="button" class="triage-preview-button triage-preview-button--keep"
        :class="{ active: triageStatus === 'keep' }" title="Keep (K)"
        aria-label="Keep" @click="setTriageStatus('keep')">
        <i class="material-symbols">star</i>
      </button>
      <button type="button" class="triage-preview-button triage-preview-button--maybe"
        :class="{ active: triageStatus === 'maybe' }" title="Maybe (M)"
        aria-label="Maybe" @click="setTriageStatus('maybe')">
        <i class="material-symbols">help</i>
      </button>
      <button type="button" class="triage-preview-button triage-preview-button--reject"
        :class="{ active: triageStatus === 'reject' }" title="Reject (X)"
        aria-label="Reject" @click="setTriageStatus('reject')">
        <i class="material-symbols">close</i>
      </button>
      <button v-if="triageStatus" type="button" class="triage-preview-button"
        title="Clear mark (0)" aria-label="Clear mark" @click="setTriageStatus('')">
        <i class="material-symbols">restart_alt</i>
      </button>
    </div>
    <!-- Loading overlay during navigation transition -->
    <div v-if="isTransitioning" class="transition-loading">
      <LoadingSpinner size="medium" />
    </div>
    <div class="preview" :class="{
      'plyr-background-light': !isDarkMode && previewType === 'audio',
      'plyr-background-dark': isDarkMode && previewType === 'audio',
      'transitioning': isTransitioning
    }" v-if="!isDeleted">
      <ExtendedImage v-if="showImage && !isTransitioning" :src="raw" @navigate-previous="navigatePrevious"
        @navigate-next="navigateNext" @close-preview="exitPreviewFromImageGesture" />

      <!-- Media: load full metadata + album art from media API before mounting plyr so the correct view/art is stable. -->
      <div v-else-if="previewType === 'audio' || previewType === 'video'" class="av-preview-wrap">
        <div v-if="avMetadataLoading" class="av-preview-loading">
          <LoadingSpinner size="medium" />
        </div>
        <plyrViewer v-else
          :key="req.path"
          ref="plyrViewer"
          :previewType="previewType"
          :raw="raw"
          :subtitlesList="subtitlesList"
          :lyrics="lyrics"
          :req="req"
          :listing="listing"
          :autoPlayEnabled="autoPlay"
          @play="playbackStarted = true"
          :class="{ 'plyr-background': previewType === 'audio' }"
          @navigate-previous="navigatePrevious"
          @navigate-next="navigateNext"
          @close-preview="exitPreviewFromImageGesture"
        />
      </div>

      <div v-else-if="isPdf" class="pdf-wrapper">
        <div v-if="usePdfPreviewFallback" class="info">
          <div class="title">
            <i class="material-symbols">picture_as_pdf</i>
            {{ $t("files.mobilePdfPreviewHint") }}
          </div>
          <div class="preview-buttons" v-if="permissions.download">
            <a
              target="_blank"
              :href="downloadUrl"
              class="button button--flat preview-action--icon-only"
              :aria-label="$t('general.download', { suffix: '' })"
            >
              <i class="material-symbols">file_download</i>
            </a>
          </div>
        </div>
        <iframe
          v-else
          allow="web-share"
          class="pdf"
          :src="raw"
          :title="req.name || 'PDF'"
        ></iframe>
      </div>

      <div v-else class="info">
        <div class="title">
          <i class="material-symbols">feedback</i>
          {{ $t("files.noPreview") }}
        </div>
        <div class="preview-buttons" v-if="permissions.download">
          <a
            target="_blank"
            :href="downloadUrl"
            class="button button--flat preview-action--icon-only"
            :aria-label="$t('general.download', { suffix: '' })"
          >
            <i class="material-symbols">file_download</i>
          </a>
        </div>
        <div v-else>
          <p> {{ $t("files.noDownloadAccess") }} </p>
        </div>
      </div>
    </div>
  </div>
</template>
<script>
import { createAsyncComponent } from "@/utils/asyncComponent.js";
import { resourcesApi, mediaApi, usersApi } from "@/api";
import { ensureViewToken, requestViewIdentity } from "@/api/viewToken.js";
import { goToItem, removeTrailingSlash, removeLastDir } from "@/utils/url.js";
import LoadingSpinner from "@/components/LoadingSpinner.vue";
import { state, getters, mutations } from "@/store";
import { isRawImageMimeType } from "@/utils/mimetype";
import { convertToVTT, getSubtitleFormatExtension } from "@/utils/subtitles";
import { parseLyrics } from "@/utils/lyrics";
import { globalVars } from "@/utils/constants";
import { navigatePlaybackQueue } from "@/utils/playbackQueue.js";
import { shouldAutoPlayPreview } from "@/utils/previewAutoplay.js";
import {
  hasActiveSession as hasActivePipSession,
  pendingInlineResumeFor,
} from "@/plyr/pipSession.js";
import { isPdfFile } from "@/utils/mediaFile";
import { shouldUsePdfPreviewFallback } from "@/utils/pdfPreview.js";
import { notify } from "@/notify";

export default {
  name: "preview",
  components: {
    LoadingSpinner,
    ExtendedImage: createAsyncComponent(() => import('@/components/files/ExtendedImage.vue')),
    plyrViewer: createAsyncComponent(() => import('@/views/files/plyrViewer.vue')),
  },
  data() {
    return {
      listing: null,
      name: "",
      fullSize: true,
      currentPrompt: null, // Replaces Vuex getter `currentPrompt`
      subtitlesList: [],
      lyrics: [],
      lyricsFetchedForPath: null,
      isDeleted: false,
      tapTimeout: null,
      avMetadataLoading: false,
      /** Skip duplicate media-metadata fetch when patchRequestFileMediaMetadata updates `req` for same path. */
      mediaEnrichDoneForPath: null,
      listingKey: null,
      /** User pressed play; enables autoplay for queue navigation even when autoplayMedia pref is off. */
      playbackStarted: false,
    };
  },
  computed: {
    permissions() {
      return getters.sourcePermissions();
    },
    showTriageControls() {
      return getters.isLoggedIn() && !getters.isShare();
    },
    triageStatus() {
      return state.req?.triageStatus || "";
    },
    showImage() {
      if (state.req.type === "image/heic" || state.req.type === "image/heif") {
        return this.isHeicAndViewable;
      }
      if (isRawImageMimeType(state.req.type)) {
        return true;
      }
      return this.previewType === 'image' || this.pdfConvertable;
    },
    autoPlay() {
      return shouldAutoPlayPreview(
        getters.previewPerms().autoplayMedia,
        this.playbackStarted,
        getters.isPreviewPlaybackQueueNavMode(),
      );
    },
    isMobileSafari() {
      const userAgent = window.navigator.userAgent;
      const isIOS =
        /iPad|iPhone|iPod/.test(userAgent) && !window.MSStream;
      const isSafari = /^((?!chrome|android).)*safari/i.test(userAgent);
      return isIOS && isSafari;
    },
    // Viewable when we can get embedded/original preview: (media + heic conversion) or Safari native
    isHeicAndViewable() {
      if (state.isSafari) return true;
      if (globalVars.mediaAvailable && globalVars.enableHeicConversion) return true;
      return false;
    },
    pdfConvertable() {
      if (!globalVars.muPdfAvailable) {
        return false;
      }
      const ext = `.${state.req.name.split(".").pop().toLowerCase()}`; // Ensure lowercase and dot
      if (state.user.disableViewingExt.includes(ext)) {
        return false;
      }
      switch (ext) {
        case '.xps':
        case '.mobi':
        case '.fb2':
        case '.cbz':
        case '.svg':
        case '.docx':
        case '.pptx':
        case '.xlsx':
        case '.hwp':
        case '.hwpx':
          return true;
        default:
          return false;
      }
    },
    sidebarShowing() {
      return getters.isSidebarVisible();
    },
    previewType() {
      if (getters.fileViewingDisabled(state.req.name)) {
        return "preview";
      }
      return getters.previewType();
    },
    isPdf() {
      return (
        isPdfFile(state.req.type) || isPdfFile(state.req.name)
      );
    },
    usePdfPreviewFallback() {
      return this.isPdf && shouldUsePdfPreviewFallback();
    },
    raw() {
      const viewToken = state.req.viewToken;
      const typeHint = state.req.type || state.req.name;
      const isHeicOrHeif = state.req.type === "image/heic" || state.req.type === "image/heif";

      if (state.isSafari && isHeicOrHeif) {
        if (getters.isShare()) {
          return resourcesApi.getViewURL(
            state.req.source,
            state.req.path,
            viewToken,
            {
              path: state.shareInfo.subPath,
              hash: state.shareInfo.hash,
            },
            false,
            typeHint,
          );
        }
        return resourcesApi.getViewURL(state.req.source, state.req.path, viewToken, null, false, typeHint);
      }

      const getRawPreview = isRawImageMimeType(state.req.type);
      const getHeicPreview = isHeicOrHeif && globalVars.mediaAvailable && globalVars.enableHeicConversion;
      if (this.pdfConvertable || getRawPreview || getHeicPreview) {
        if (getters.isShare()) {
          const previewPath = removeTrailingSlash(state.req.path);
          return resourcesApi.getPreviewURLPublic(previewPath, "original", state.req.viewToken);
        }
        return (
          `${resourcesApi.getPreviewURL(
            state.req.source,
            state.req.path,
            state.req.modified,
          )}&size=original`
        );
      }
      if (getters.isShare()) {
        return resourcesApi.getViewURL(
          state.req.source,
          state.req.path,
        viewToken,
          {
            path: state.shareInfo.subPath,
            hash: state.shareInfo.hash,
          },
          false,
          typeHint,
        );
      }
      return resourcesApi.getViewURL(state.req.source, state.req.path, viewToken, null, false, typeHint);
    },
    isDarkMode() {
      return getters.isDarkMode();
    },
    downloadUrl() {
      if (getters.isShare()) {
        return resourcesApi.getDownloadURLPublic(
          {
            path: state.shareInfo.subPath,
            hash: state.shareInfo.hash,
          },
          [state.req.path],
        );
      }
      return resourcesApi.getDownloadURL(state.req.source, state.req.path);
    },
    isTransitioning() {
      return state.navigation.isTransitioning;
    },
    getSubtitles() {
      return this.subtitles();
    },
    req() {
      return state.req;
    },
    disableFileViewer() {
      return state.shareInfo.disableFileViewer;
    },
  },
  watch: {
    req: {
      immediate: true,
      async handler() {
        await this.loadPreviewForReq();
      },
    },
  },
  mounted() {
    window.addEventListener("keydown", this.keyEvent);
  },
  beforeUnmount() {
    window.removeEventListener("keydown", this.keyEvent);
    this.$refs.plyrViewer?.destroyPlyr?.();
    // Clear navigation state when leaving preview
    mutations.clearNavigation();
  },
  methods: {
    async setTriageStatus(requestedStatus) {
      if (!this.showTriageControls || !state.req?.name) return;
      const current = state.req.triageStatus || "";
      const nextStatus = requestedStatus && current === requestedStatus ? "" : requestedStatus;
      const directoryPath = removeLastDir(state.req.path) || "/";
      try {
        await usersApi.patchTriageItem({
          source: state.req.source,
          path: directoryPath,
          name: state.req.name,
          status: nextStatus,
        });
        state.req.triageStatus = nextStatus;
        const item = Array.isArray(this.listing)
          ? this.listing.find((entry) => entry.path === state.req.path)
          : null;
        if (item) {
          item.triageStatus = nextStatus;
        }
      } catch (error) {
        notify.showError(error);
      }
    },
    async attachDirMediaMetadata(listing, dirPath) {
      if (!listing?.length) return;
      try {
        const isShare = getters.isShare();
        const metaMap = isShare
          ? await mediaApi.getDirectoryMetadataMap(dirPath, {
              isShare: true,
              hash: state.shareInfo.hash,
              password: localStorage.getItem(`sharepass:${state.shareInfo.hash}`) || '',
            })
          : await mediaApi.getDirectoryMetadataMap(dirPath, { source: state.req.source });
        if (this.listing !== listing) return;
        mutations.patchListingMetadata(listing, metaMap, state.req.path);
      } catch (e) {
        console.warn('dir items metadata fetch failed', e);
      }
    },
    listingContextKey(directoryPath) {
      return getters.isShare()
        ? `share:${state.shareInfo?.hash || ""}:${directoryPath}`
        : `source:${state.req.source || ""}:${directoryPath}`;
    },
    async loadPreviewForReq() {
      if (!getters.isLoggedIn() && !getters.isShare()) {
        return;
      }
      if (!getters.isPreviewPlaybackQueueNavMode()) {
        this.playbackStarted = false;
      }
      this.isDeleted = false;
      const currentDirectoryPath = removeLastDir(state.req.path) || '/';
      const currentListingKey = this.listingContextKey(currentDirectoryPath);
      if (this.listingKey !== currentListingKey) {
        this.listing = null;
      }

      const path = state.req.path;
      if (
        state.req.type !== "directory"
        && !getters.fileViewingDisabled(state.req.name)
      ) {
        const viewIdentity = requestViewIdentity(state.req);
        try {
          const viewToken = await ensureViewToken(state.req.source);
          if (viewToken && requestViewIdentity(state.req) === viewIdentity) {
            mutations.setRequestViewToken(viewToken);
          }
        } catch (err) {
          console.warn("Failed to refresh view token for preview:", err);
        }
      }

      if (!this.listing || this.listing === "undefined") {
        if (state.req.parentDirItems) {
          this.listing = state.req.parentDirItems;
          this.listingKey = currentListingKey;
        } else if (state.req.items) {
          this.listing = state.req.items;
          this.listingKey = currentListingKey;
        }
      }

      const isAv =
        !getters.fileViewingDisabled(state.req.name) &&
        state.req.type !== "directory" &&
        (this.previewType === "audio" || this.previewType === "video");

      if (!isAv) {
        this.avMetadataLoading = false;
        this.mediaEnrichDoneForPath = null;
        this.lyricsFetchedForPath = null;
      } else {
        const pipHandoffForThisFile =
          hasActivePipSession(state.req.source, state.req.path)
          || pendingInlineResumeFor(state.req.source, state.req.path);
        if (this.mediaEnrichDoneForPath !== path) {
          this.mediaEnrichDoneForPath = path;
          if (!pipHandoffForThisFile) {
            this.avMetadataLoading = true;
          }
          try {
            await this.enrichAvFromMediaApi(path);
            if (state.req.path !== path) {
              return;
            }
          } finally {
            if (state.req.path === path) {
              this.avMetadataLoading = false;
            }
          }
        } else {
          this.avMetadataLoading = false;
        }
      }

      if (state.req.path !== path) {
        return;
      }
      await this.updatePreview();
      if (isAv && this.listing) {
        const directoryPath = removeLastDir(state.req.path) || '/';
        await this.attachDirMediaMetadata(this.listing, directoryPath);
      }
      this.subtitlesList = await this.subtitles();
      if (this.previewType === 'audio' && this.lyricsFetchedForPath !== state.req.path) {
        this.lyricsFetchedForPath = state.req.path;
        if (state.req.metadata?.hasLyrics) {
          try {
            if (getters.isShare()) {
              const hash = state.shareInfo.hash;
              const password = localStorage.getItem(`sharepass:${hash}`) || "";
              const { lyrics: raw, format } = await mediaApi.getLyricsPublic(state.req.path, hash, password);
              this.lyrics = parseLyrics(raw, format);
            } else {
              const { lyrics: raw, format } = await mediaApi.getLyrics(state.req.source, state.req.path);
              this.lyrics = parseLyrics(raw, format);
            }
          } catch (err) {
            console.warn("Failed to fetch lyrics:", err);
            this.lyrics = [];
          }
        } else {
          this.lyrics = [];
        }
      }
      mutations.resetSelected();
      mutations.addSelected({
        name: state.req.name,
        path: state.req.path,
        size: state.req.size,
        type: state.req.type,
        source: state.req.source,
        modified: state.req.modified,
        hasPreview: state.req.hasPreview,
      });
    },
    /** GET /api/media/metadata?albumArt=true — subtitles, duration, embedded cover for plyr. */
    async enrichAvFromMediaApi(expectedPath) {
      if (state.req.path !== expectedPath) {
        return;
      }
      const req = state.req;
      try {
        let enriched;
        if (getters.isShare()) {
          const pwd =
            localStorage.getItem(`sharepass:${state.shareInfo.hash}`) || "";
          enriched = await mediaApi.fetchDirectoryMediaMetadataPublic(
            req.path,
            state.shareInfo.hash,
            pwd,
            true,
          );
        } else {
          if (!getters.isLoggedIn()) {
            return;
          }
          enriched = await mediaApi.fetchDirectoryMediaMetadata(
            req.source,
            req.path,
            true,
          );
        }
        if (state.req.path !== expectedPath) {
          return;
        }
        if (enriched && enriched.type !== "directory") {
          mutations.patchRequestFileMediaMetadata(enriched);
        }
      } catch (e) {
        console.warn("Preview: media metadata fetch failed", e);
      }
    },
    async subtitles() {
      if (!state.req.subtitles?.length) {
        return [];
      }
      const subs = [];
      // Fetch subtitle content for each track using the media API
      for (let index = 0; index < state.req.subtitles.length; index++) {
        const subtitleTrack = state.req.subtitles.at(index);
        try {
          // Fetch subtitle content from API using name and embedded
          const content = await mediaApi.getSubtitleContent(
            state.req.source,
            state.req.path,
            subtitleTrack.name,
            subtitleTrack.embedded,
          );
          if (!content || content.length === 0) {
            console.warn("Subtitle track has no content:", subtitleTrack.name);
            continue;
          }
          // Convert to VTT if needed
          let vttContent = content;
          if (!content.startsWith("WEBVTT")) {
            const ext = getSubtitleFormatExtension(subtitleTrack.name);
            vttContent = convertToVTT(ext, content);
          }
          if (vttContent.startsWith("WEBVTT")) {
            // Create a virtual file (Blob) and get a URL for it
            const blob = new Blob([vttContent], { type: "text/vtt" });
            const vttURL = URL.createObjectURL(blob);

            subs.push({
              name: subtitleTrack.name,
              src: vttURL,
              srclang: subtitleTrack.srclang || subtitleTrack.language,
            });
          } else {
            console.warn(
              "Skipping subtitle track - no WEBVTT header after conversion:",
              subtitleTrack.name
            );
          }
        } catch (err) {
          console.error("Failed to load subtitle:", subtitleTrack.name, err);
        }
      }
      return subs;
    },
    async keyEvent(event) {
      if (getters.currentPromptName() || event.repeat) {
        return;
      }

      const { key, altKey } = event;
      const target = event.target;
      const tag = target?.tagName?.toLowerCase?.() || "";
      const typing = tag === "input" || tag === "textarea" || tag === "select" || target?.isContentEditable;
      if (!typing && this.showTriageControls && !altKey) {
        switch (key.toLowerCase()) {
          case "k":
            event.preventDefault();
            await this.setTriageStatus("keep");
            return;
          case "m":
            event.preventDefault();
            await this.setTriageStatus("maybe");
            return;
          case "x":
            event.preventDefault();
            await this.setTriageStatus("reject");
            return;
          case "0":
            event.preventDefault();
            await this.setTriageStatus("");
            return;
        }
      }

      let shortcut = key;
      if (altKey) shortcut = `Alt+${key}`;

      switch (shortcut) {
        case "Alt+ArrowUp":
          event.preventDefault();
          // fall through
        case "Escape":
        case "Backspace":
          this.close();
          break;
        case "Delete":
          this.showDeletePrompt();
          break;
      }
    },
    async updatePreview() {
      const expectedPath = state.req.path;
      let directoryPath = removeLastDir(state.req.path);

      // If directoryPath is empty, the file is in root - use '/' as the directory
      if (!directoryPath || directoryPath === '') {
        directoryPath = '/';
      }
      const expectedListingKey = this.listingContextKey(directoryPath);

      if (!this.listing || this.listing === "undefined") {
        // Try to use pre-fetched parent directory items first
        if (state.req.parentDirItems) {
          this.listing = state.req.parentDirItems;
        } else if (directoryPath !== state.req.path) {
          // Fetch directory listing (now with '/' for root files)
          try {
            let res;
            if (getters.isShare()) {
              // Use public API for shared files
              res = await resourcesApi.fetchFilesPublic(
                directoryPath,
                state.shareInfo?.hash,
              );
            } else {
              // Use regular files API for authenticated users
              res = await resourcesApi.fetchFiles(
                state.req.source,
                directoryPath,
              );
            }
            if (state.req.path !== expectedPath || this.listingContextKey(directoryPath) !== expectedListingKey) return;
            this.listing = res.items;
          } catch (error) {
            if (state.req.path !== expectedPath) return;
            console.error("error Preview.vue", error);
            this.listing = [state.req];
          }
        } else {
          this.listing = [state.req];
        }
      }

      if (!this.listing) {
        this.listing = [state.req];
      }
      this.listingKey = this.listingContextKey(directoryPath);
      this.name = state.req.name;

      // Setup navigation using the new state management
      mutations.setupNavigation({
        listing: this.listing,
        currentItem: state.req,
        directoryPath: directoryPath
      });
    },

    prefetchUrl(item) {
      const viewToken = item.viewToken || state.req.viewToken;
      const typeHint = item.type || item.name;
      if (getters.isShare()) {
        return this.fullSize
          ? resourcesApi.getViewURL(
              state.req.source,
              item.path,
              viewToken,
              {
                path: item.path,
                hash: state.shareInfo?.hash,
              },
              false,
              typeHint,
            )
          : resourcesApi.getPreviewURLPublic(item.path);
      }
      return this.fullSize
        ? resourcesApi.getViewURL(state.req.source, item.path, viewToken, null, false, typeHint)
        : resourcesApi.getPreviewURL(
            state.req.source,
            item.path,
            item.modified,
          );
    },
    resetPrompts() {
      this.currentPrompt = null;
    },
    showDeletePrompt() {
      const item = state.req;
      const previewUrl = item.hasPreview
        ? resourcesApi.getPreviewURL(item.source, item.path, item.modified)
        : null;
      mutations.showPrompt({
        name: "delete",
        props: {
          items: [{
            source: item.source,
            path: item.path,
            type: item.type,
            size: item.size,
            modified: item.modified,
            hasPreview: item.hasPreview,
            previewUrl: previewUrl,
          }],
        },
      });
    },
    toggleSize() {
      this.fullSize = !this.fullSize;
    },
    close() {
      mutations.replaceRequest({}); // Reset request data
      const uri = `${removeLastDir(state.route.path)}/`;
      this.$router.push({ path: uri });
    },
    download() {
      const items = [state.req];
      downloadFiles(items);
    },
    navigatePrevious() {
      if (getters.isPreviewPlaybackQueueNavMode()) {
        if (!getters.playbackQueueCanGoPrevious()) {
          return;
        }
        navigatePlaybackQueue(-1);
        return;
      }
      if (state.navigation.previousLink) {
        mutations.setNavigationTransitioning(true);
        this.$router.replace({ path: state.navigation.previousLink });
      }
    },
    navigateNext() {
      if (getters.isPreviewPlaybackQueueNavMode()) {
        if (!getters.playbackQueueCanGoNext()) {
          return;
        }
        navigatePlaybackQueue(1);
        return;
      }
      if (state.navigation.nextLink) {
        mutations.setNavigationTransitioning(true);
        this.$router.replace({ path: state.navigation.nextLink });
      }
    },
    /** Same navigation as header “back” in preview (Default.vue performNavigation). */
    exitPreviewFromImageGesture() {
      mutations.closeHovers();
      if (state.previousHistoryItem?.name) {
        goToItem(
          state.previousHistoryItem.source,
          state.previousHistoryItem.path,
          state.previousHistoryItem,
          false,
          state.previousHistoryItem.isShare
        );
        return;
      }
      const parentPath = removeLastDir(state.route.path);
      this.$router.push({ path: parentPath });
    },
  },
};
</script>

<style scoped>
/* Loading overlay for navigation transitions */
.transition-loading {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: var(--background);
  z-index: 10000;
  transition: opacity 0.1s ease;
}

.transition-loading .spinner {
  width: 70px;
  text-align: center;
}

.transition-loading .spinner>div {
  width: 18px;
  height: 18px;
  background-color: var(--textPrimary);
  border-radius: 100%;
  display: inline-block;
  animation: sk-bouncedelay 1.4s infinite ease-in-out both;
}

.transition-loading .spinner .bounce1 {
  animation-delay: -0.32s;
}

.transition-loading .spinner .bounce2 {
  animation-delay: -0.16s;
}

@keyframes sk-bouncedelay {
  0%, 80%, 100% {
    transform: scale(0);
  }
  40% {
    transform: scale(1.0);
  }
}

.preview.transitioning {
  opacity: 0.3;
  pointer-events: none;
}

.pdf-wrapper {
  position: relative;
  width: 100%;
  height: 100%;
}

.pdf-wrapper .pdf {
  width: 100%;
  height: 100%;
  border: 0;
}

.pdf-wrapper .floating-btn {
  background: rgb(0 0 0 / 50%);
  color: white;
}

.pdf-wrapper .floating-btn:hover {
  background: rgb(0 0 0 / 70%);
}

.preview-buttons {
  display: flex;
  flex-direction: row;
  gap: 1em;
}

.av-preview-wrap {
  width: 100%;
  height: 100%;
  min-height: 12rem;
}

.av-preview-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 12rem;
}


.triage-preview-controls {
  position: fixed;
  top: 4.25em;
  right: 1em;
  z-index: 80;
  display: flex;
  gap: 0.35em;
  padding: 0.35em;
  border-radius: 999px;
  background: color-mix(in srgb, var(--background) 86%, transparent);
  box-shadow: 0 2px 12px rgb(0 0 0 / 28%);
  backdrop-filter: blur(10px);
}

.triage-preview-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.8em;
  height: 2.8em;
  min-width: 2.8em;
  padding: 0;
  border: 0;
  border-radius: 50%;
  color: var(--textSecondary);
  background: transparent;
  cursor: pointer;
  touch-action: manipulation;
}

.triage-preview-button.active {
  color: var(--primaryColor);
  background: color-mix(in srgb, var(--primaryColor) 20%, var(--background));
  font-variation-settings: 'FILL' 1;
}

.triage-preview-button--reject.active {
  color: var(--red);
  background: color-mix(in srgb, var(--red) 18%, var(--background));
}

.triage-preview-button--maybe.active {
  color: var(--orange);
  background: color-mix(in srgb, var(--orange) 18%, var(--background));
}

@media (pointer: coarse) {
  .triage-preview-button {
    width: 3.2em;
    height: 3.2em;
    min-width: 3.2em;
  }
}

</style>
