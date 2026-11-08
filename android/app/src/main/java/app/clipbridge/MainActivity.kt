package app.clipbridge

import android.app.Activity
import android.content.ClipData
import android.content.ClipboardManager
import android.content.ContentValues
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Color
import android.net.Uri
import android.os.Bundle
import android.os.Environment
import android.provider.MediaStore
import android.text.InputType
import android.view.Gravity
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

class MainActivity : Activity() {

    private var server: String = ""
    private lateinit var status: TextView
    private lateinit var hostField: EditText
    private lateinit var preview: TextView
    private lateinit var imageView: ImageView
    private lateinit var saveButton: Button

    private var lastImage: Bitmap? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(48, 64, 48, 48)
            setBackgroundColor(Color.parseColor("#0e1116"))
        }

        val title = TextView(this).apply {
            text = "ClipBridge"
            textSize = 24f
            setTextColor(Color.parseColor("#e6edf3"))
        }

        status = TextView(this).apply {
            text = "Looking for the host"
            textSize = 14f
            setTextColor(Color.parseColor("#8b98a8"))
            setPadding(0, 8, 0, 24)
        }

        hostField = EditText(this).apply {
            hint = "http://192.168.1.2:1717"
            inputType = InputType.TYPE_TEXT_VARIATION_URI
            setTextColor(Color.parseColor("#e6edf3"))
            setHintTextColor(Color.parseColor("#5c6a7a"))
            textSize = 14f
        }

        val applyButton = button("Use this address") { useAddress() }
        val sendButton = button("Send my clipboard") { sendClipboard() }
        val pullButton = button("Get from host") { pull() }

        preview = TextView(this).apply {
            text = ""
            textSize = 14f
            setTextColor(Color.parseColor("#e6edf3"))
            setPadding(0, 24, 0, 12)
        }

        imageView = ImageView(this).apply {
            adjustViewBounds = true
            visibility = android.view.View.GONE
        }

        saveButton = button("Save image") { saveImage() }
        saveButton.visibility = android.view.View.GONE

        listOf(title, status, hostField, applyButton, sendButton, pullButton, preview, imageView, saveButton)
            .forEach { root.addView(it) }

        setContentView(ScrollView(this).apply { addView(root) })

        Thread { discover() }.start()
    }

    private fun button(label: String, action: () -> Unit): Button {
        return Button(this).apply {
            text = label
            setOnClickListener { action() }
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT
            ).apply { setMargins(0, 8, 0, 8) }
        }
    }

    private fun setStatus(text: String, ok: Boolean) {
        runOnUiThread {
            status.text = text
            status.setTextColor(Color.parseColor(if (ok) "#22c55e" else "#ef4444"))
        }
    }

    /** Listens for the host announcement on UDP 1719. */
    private fun discover() {
        try {
            val socket = java.net.DatagramSocket(1719)
            socket.broadcast = true
            socket.soTimeout = 15000
            val buf = ByteArray(2048)
            val deadline = System.currentTimeMillis() + 15000
            while (System.currentTimeMillis() < deadline) {
                val packet = java.net.DatagramPacket(buf, buf.size)
                socket.receive(packet)
                val text = String(packet.data, 0, packet.length)
                val json = JSONObject(text)
                if (json.optString("app") == "clipbridge") {
                    val port = json.optInt("port", 0)
                    val ip = packet.address.hostAddress ?: continue
                    if (port > 0 && server.isEmpty()) {
                        server = "http://$ip:$port"
                        runOnUiThread {
                            hostField.setText(server)
                            status.text = "Connected to $server"
                            status.setTextColor(Color.parseColor("#22c55e"))
                        }
                        pull()
                        return
                    }
                }
            }
            if (server.isEmpty()) setStatus("No host found. Type its address and press Use.", false)
        } catch (e: Exception) {
            if (server.isEmpty()) setStatus("Search stopped: ${e.message}", false)
        }
    }

    private fun useAddress() {
        val typed = hostField.text.toString().trim().trimEnd('/')
        if (typed.isEmpty()) return
        server = if (typed.startsWith("http")) typed else "http://$typed"
        hostField.setText(server)
        setStatus("Using $server", true)
        pull()
    }

    private fun get(path: String): ByteArray? {
        return try {
            val conn = URL(server + path).openConnection() as HttpURLConnection
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            conn.inputStream.use { it.readBytes() }
        } catch (e: Exception) {
            null
        }
    }

    private fun pull() {
        if (server.isEmpty()) {
            setStatus("No host yet", false)
            return
        }
        Thread {
            val state = get("/state") ?: run {
                setStatus("Host not answering", false)
                return@Thread
            }
            val json = JSONObject(String(state))
            val type = json.optString("type", "")
            when (type) {
                "text" -> {
                    val text = json.optString("text", "")
                    setStatus("Got text", true)
                    runOnUiThread {
                        preview.text = text
                        imageView.visibility = android.view.View.GONE
                        saveButton.visibility = android.view.View.GONE
                        copyToClipboard(text)
                        Toast.makeText(this, "Copied to your clipboard", Toast.LENGTH_SHORT).show()
                    }
                }
                "image" -> {
                    val bytes = get("/image")
                    if (bytes == null) {
                        setStatus("Image download failed", false)
                        return@Thread
                    }
                    val bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                    if (bitmap == null) {
                        setStatus("Image could not be read", false)
                        return@Thread
                    }
                    lastImage = bitmap
                    setStatus("Got image", true)
                    runOnUiThread {
                        preview.text = ""
                        imageView.setImageBitmap(bitmap)
                        imageView.visibility = android.view.View.VISIBLE
                        saveButton.visibility = android.view.View.VISIBLE
                    }
                }
                else -> setStatus("Host has nothing yet", false)
            }
        }.start()
    }

    private fun sendClipboard() {
        if (server.isEmpty()) {
            setStatus("No host yet", false)
            return
        }
        val cm = getSystemService(CLIPBOARD_SERVICE) as ClipboardManager
        val clip = cm.primaryClip
        if (clip == null || clip.itemCount == 0) {
            setStatus("Nothing on the clipboard", false)
            return
        }
        val text = clip.getItemAt(0).coerceToText(this)?.toString() ?: ""
        if (text.isEmpty()) {
            setStatus("Nothing to send", false)
            return
        }
        Thread {
            try {
                val conn = URL(server + "/set").openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.doOutput = true
                conn.setRequestProperty("Content-Type", "application/json")
                conn.connectTimeout = 5000
                conn.readTimeout = 5000
                val body = JSONObject().put("text", text).toString().toByteArray()
                conn.outputStream.use { it.write(body) }
                val code = conn.responseCode
                conn.disconnect()
                setStatus(if (code == 200) "Sent to host" else "Host said $code", code == 200)
            } catch (e: Exception) {
                setStatus("Send failed: ${e.message}", false)
            }
        }.start()
    }

    private fun copyToClipboard(text: String) {
        val cm = getSystemService(CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(ClipData.newPlainText("ClipBridge", text))
    }

    private fun saveImage() {
        val bitmap = lastImage ?: return
        try {
            val name = "clipbridge-${System.currentTimeMillis()}.png"
            val values = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, name)
                put(MediaStore.MediaColumns.MIME_TYPE, "image/png")
                put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
            }
            val resolver = contentResolver
            val uri: Uri? = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
            if (uri != null) {
                resolver.openOutputStream(uri)?.use { out ->
                    bitmap.compress(Bitmap.CompressFormat.PNG, 100, out)
                }
                Toast.makeText(this, "Saved to Downloads", Toast.LENGTH_SHORT).show()
            }
        } catch (e: Exception) {
            Toast.makeText(this, "Save failed: ${e.message}", Toast.LENGTH_LONG).show()
        }
    }
}
