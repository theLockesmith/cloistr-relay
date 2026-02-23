// nostr-auth.js - NIP-07 and NIP-46 authentication helper for Cloistr Relay Admin

// Global bunker signer instance (for NIP-46)
let bunkerSigner = null;
let bunkerPool = null;

// Check if NIP-07 extension is available
function hasNostrExtension() {
    return typeof window.nostr !== 'undefined';
}

// Get the current auth method
function getAuthMethod() {
    return sessionStorage.getItem('nostr_auth_method') || 'nip07';
}

// Get the user's public key
async function getPublicKey() {
    const method = getAuthMethod();

    if (method === 'nip46' && bunkerSigner) {
        return await bunkerSigner.getPublicKey();
    }

    if (hasNostrExtension()) {
        return await window.nostr.getPublicKey();
    }

    throw new Error('No signer available. Please log in again.');
}

// Initialize NIP-46 bunker connection
async function initBunkerConnection() {
    const bunkerUrl = sessionStorage.getItem('nip46_bunker_url');
    const clientSk = sessionStorage.getItem('nip46_client_sk');

    if (!bunkerUrl || !clientSk || !window.nostrTools) {
        return false;
    }

    try {
        const { BunkerSigner, parseBunkerInput, SimplePool } = window.nostrTools;

        const bunkerPointer = await parseBunkerInput(bunkerUrl);
        if (!bunkerPointer) {
            return false;
        }

        bunkerPool = new SimplePool();
        bunkerSigner = BunkerSigner.fromBunker(
            clientSk,
            bunkerPointer,
            { pool: bunkerPool }
        );

        await bunkerSigner.connect();
        return true;
    } catch (err) {
        console.error('Failed to init bunker connection:', err);
        return false;
    }
}

// Create a NIP-98 HTTP Auth event and sign it
async function createAuthHeader(method, url) {
    const authMethod = getAuthMethod();

    // Create unsigned event (kind 27235 = NIP-98 HTTP Auth)
    const event = {
        kind: 27235,
        created_at: Math.floor(Date.now() / 1000),
        tags: [
            ['u', url],
            ['method', method.toUpperCase()]
        ],
        content: ''
    };

    let signedEvent;

    if (authMethod === 'nip46') {
        // Ensure bunker is connected
        if (!bunkerSigner) {
            const connected = await initBunkerConnection();
            if (!connected) {
                throw new Error('Failed to connect to bunker. Please log in again.');
            }
        }

        // Add pubkey and sign with bunker
        event.pubkey = await bunkerSigner.getPublicKey();
        signedEvent = await bunkerSigner.signEvent(event);
    } else {
        // NIP-07 extension
        if (!hasNostrExtension()) {
            throw new Error('No Nostr browser extension found. Install nos2x, Alby, or similar.');
        }
        signedEvent = await window.nostr.signEvent(event);
    }

    // Base64 encode for Authorization header
    return 'Nostr ' + btoa(JSON.stringify(signedEvent));
}

// Make an authenticated request
async function signedRequest(method, path, formData) {
    const url = window.location.origin + path;

    try {
        const authHeader = await createAuthHeader(method, url);

        // Convert FormData to URLSearchParams for POST body
        const body = new URLSearchParams();
        for (const [key, value] of formData.entries()) {
            body.append(key, value);
        }

        const response = await fetch(url, {
            method: method,
            headers: {
                'Authorization': authHeader,
                'Content-Type': 'application/x-www-form-urlencoded',
                'HX-Request': 'true'
            },
            body: body.toString()
        });

        // Handle response
        const html = await response.text();

        // If response contains a toast, add it to the container
        if (html.includes('toast-enter')) {
            document.getElementById('toast-container').innerHTML += html;
        }

        // Trigger refresh events if header present
        const trigger = response.headers.get('HX-Trigger');
        if (trigger) {
            htmx.trigger(document.body, trigger);
        }

        if (!response.ok) {
            throw new Error('Request failed: ' + response.status);
        }

        return response;
    } catch (err) {
        console.error('Signed request failed:', err);

        // If auth failed, redirect to login
        if (err.message.includes('log in again') || err.message.includes('signer')) {
            sessionStorage.clear();
            window.location.href = '/login';
            return;
        }

        throw err;
    }
}

// Show a toast notification
function showToast(type, message) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast-enter p-4 rounded-lg shadow-lg ${type === 'error' ? 'bg-red-600' : 'bg-green-600'} text-white text-sm`;
    toast.innerHTML = `
        <div class="flex items-center">
            ${type === 'error'
                ? '<svg class="h-5 w-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>'
                : '<svg class="h-5 w-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>'
            }
            <span>${message}</span>
        </div>
    `;
    container.appendChild(toast);

    // Auto-dismiss after 5 seconds
    setTimeout(() => {
        toast.classList.remove('toast-enter');
        toast.classList.add('toast-exit');
        setTimeout(() => toast.remove(), 300);
    }, 5000);
}

// Login with Nostr extension (NIP-07)
async function loginWithNostr() {
    try {
        if (!hasNostrExtension()) {
            showToast('error', 'No Nostr browser extension found. Install nos2x, Alby, or similar.');
            return;
        }

        const pubkey = await window.nostr.getPublicKey();

        // Update UI to show logged in state
        const authPubkey = document.getElementById('auth-pubkey');
        const loginBtn = document.getElementById('login-btn');

        if (authPubkey && loginBtn) {
            authPubkey.textContent = pubkey.substring(0, 8) + '...' + pubkey.substring(pubkey.length - 8);
            loginBtn.textContent = 'Logged in';
            loginBtn.disabled = true;
            loginBtn.classList.remove('bg-nostr-purple', 'hover:bg-purple-600');
            loginBtn.classList.add('bg-gray-600', 'cursor-default');
        }

        // Store pubkey and auth method
        sessionStorage.setItem('nostr_pubkey', pubkey);
        sessionStorage.setItem('nostr_auth_method', 'nip07');

        showToast('success', 'Logged in with Nostr');

        // Hide auth notice if present
        const authNotice = document.getElementById('auth-notice');
        if (authNotice) {
            authNotice.classList.add('hidden');
        }
    } catch (err) {
        console.error('Login failed:', err);
        showToast('error', err.message);
    }
}

// Logout
async function logout() {
    // Clean up bunker connection
    if (bunkerPool && bunkerSigner) {
        const bunkerUrl = sessionStorage.getItem('nip46_bunker_url');
        if (bunkerUrl && window.nostrTools) {
            const { parseBunkerInput } = window.nostrTools;
            const pointer = await parseBunkerInput(bunkerUrl);
            if (pointer && pointer.relays) {
                bunkerPool.close(pointer.relays);
            }
        }
    }
    bunkerSigner = null;
    bunkerPool = null;

    // Clear session
    sessionStorage.removeItem('nostr_pubkey');
    sessionStorage.removeItem('nostr_auth_method');
    sessionStorage.removeItem('nip46_bunker_url');
    sessionStorage.removeItem('nip46_client_sk');

    // Redirect to login
    window.location.href = '/login';
}

// Check login state on page load
document.addEventListener('DOMContentLoaded', function() {
    const storedPubkey = sessionStorage.getItem('nostr_pubkey');
    const authMethod = getAuthMethod();

    if (storedPubkey) {
        const authPubkey = document.getElementById('auth-pubkey');
        const loginBtn = document.getElementById('login-btn');

        if (authPubkey && loginBtn) {
            const methodLabel = authMethod === 'nip46' ? ' (NIP-46)' : '';
            authPubkey.textContent = storedPubkey.substring(0, 8) + '...' + storedPubkey.substring(storedPubkey.length - 8) + methodLabel;
            loginBtn.textContent = 'Logout';
            loginBtn.disabled = false;
            loginBtn.onclick = logout;
            loginBtn.classList.remove('bg-nostr-purple', 'hover:bg-purple-600');
            loginBtn.classList.add('bg-gray-600', 'hover:bg-gray-500');
        }

        // Init bunker connection if using NIP-46
        if (authMethod === 'nip46' && window.nostrTools) {
            initBunkerConnection().catch(console.error);
        }
    }

    // Check if extension is available for NIP-07
    if (!hasNostrExtension() && authMethod !== 'nip46') {
        const authNotice = document.getElementById('auth-notice');
        if (authNotice) {
            authNotice.classList.remove('hidden');
        }
    }
});

// Export for use in templates
window.hasNostrExtension = hasNostrExtension;
window.getPublicKey = getPublicKey;
window.createAuthHeader = createAuthHeader;
window.signedRequest = signedRequest;
window.showToast = showToast;
window.loginWithNostr = loginWithNostr;
window.logout = logout;
window.getAuthMethod = getAuthMethod;
