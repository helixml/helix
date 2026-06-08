// useGitHubAppActions encapsulates the GitHub App create / install / manage
// browser flows shared by the New Stream dialog and the Settings page panel.
// All three are top-level navigations to github.com in a popup; GitHub's COOP
// headers sever window.opener, so completion is detected by a postMessage from
// our callback (when available) AND a window-focus fallback. The caller passes
// onComplete to re-check install status.

import { useGitHubManifestStart } from '../../services/helixOrgService'
import useSnackbar from '../../hooks/useSnackbar'

export function useGitHubAppActions(onComplete?: () => void) {
  const snackbar = useSnackbar()
  const manifestStart = useGitHubManifestStart()

  const openPopup = (url: string, name: string): Window | null => {
    const width = 1000
    const height = 800
    const left = (window.innerWidth - width) / 2
    const top = (window.innerHeight - height) / 2
    const popup = window.open(
      url,
      name,
      `width=${width},height=${height},left=${left},top=${top},toolbar=0,location=0,menubar=0,directories=0,scrollbars=1`,
    )
    if (!popup) snackbar.error('Popup blocked — allow popups for this site and try again.')
    return popup
  }

  const watch = () => {
    const onMessage = (event: MessageEvent) => {
      if (event.data?.type === 'github-app-installed') {
        window.removeEventListener('message', onMessage)
        window.removeEventListener('focus', onFocus)
        snackbar.success('Helix installed — refreshing…')
        onComplete?.()
      } else if (event.data?.type === 'github-app-created') {
        // The app was created but not installed yet — refetch so the gate
        // advances to the Install step. (Focus fallback also covers this if
        // GitHub's COOP severed window.opener and this never arrives.)
        window.removeEventListener('message', onMessage)
        window.removeEventListener('focus', onFocus)
        snackbar.success('App created — now click "Install Helix".')
        onComplete?.()
      } else if (event.data?.type === 'github-app-install-error') {
        window.removeEventListener('message', onMessage)
        snackbar.error('GitHub reported a problem completing the install.')
      }
    }
    const onFocus = () => {
      window.removeEventListener('message', onMessage)
      onComplete?.()
    }
    window.addEventListener('message', onMessage)
    window.addEventListener('focus', onFocus, { once: true })
  }

  // createApp runs the Manifest flow: ask the backend for the GitHub POST URL +
  // manifest, then submit it as a form inside a popup so GitHub creates the app
  // (org-owned) on the user's behalf, then chains into install.
  const createApp = async (githubOrg: string) => {
    const org = githubOrg.trim()
    if (!org) {
      snackbar.error('Enter the GitHub organization to create the app under')
      return
    }
    // Open synchronously on the click — opening after the await loses the gesture.
    const popup = openPopup('about:blank', 'github-app-create')
    if (!popup) return
    try {
      popup.document.body.innerHTML = 'Preparing the Helix app…'
      const start = await manifestStart.mutateAsync({ github_org: org, origin: window.location.origin })
      const doc = popup.document
      doc.body.innerHTML = 'Redirecting to GitHub to create the Helix app…'
      const form = doc.createElement('form')
      form.method = 'POST'
      form.action = start.post_url
      const input = doc.createElement('input')
      input.type = 'hidden'
      input.name = 'manifest'
      input.value = start.manifest
      form.appendChild(input)
      doc.body.appendChild(form)
      form.submit()
      watch()
    } catch (e: any) {
      try { popup.close() } catch { /* ignore */ }
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'Could not start GitHub app creation')
    }
  }

  // installApp sends the user to GitHub to pick repos to install the app on.
  const installApp = (installUrl?: string) => {
    if (!installUrl) {
      snackbar.error('The Helix GitHub App is not configured on this deployment. Ask an admin to set GITHUB_APP_SLUG.')
      return
    }
    if (!openPopup(installUrl, 'github-app-install')) return
    watch()
  }

  // openManage opens the app's developer-settings page (edit permissions /
  // repos / delete). Plain new tab — no completion to watch.
  const openManage = (manageUrl?: string) => {
    if (!manageUrl) {
      snackbar.error('Manage URL unavailable for this app.')
      return
    }
    window.open(manageUrl, '_blank', 'noopener,noreferrer')
  }

  return { createApp, installApp, openManage, creating: manifestStart.isPending }
}
