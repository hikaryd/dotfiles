const policy = chrome.privacy.network.webRTCIPHandlingPolicy;
policy.set({value: 'disable_non_proxied_udp', scope: 'regular'}, () => {
  if (chrome.runtime.lastError) {
    console.error('WebRTC privacy policy could not be set:', chrome.runtime.lastError.message);
    return;
  }
  policy.get({}, result => {
    if (chrome.runtime.lastError || !result || result.value !== 'disable_non_proxied_udp' ||
        result.levelOfControl !== 'controlled_by_this_extension') {
      console.error('WebRTC privacy policy is not controlled by this extension');
    }
  });
});
