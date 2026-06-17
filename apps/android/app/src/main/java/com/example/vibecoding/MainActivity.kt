package com.example.vibecoding

import android.app.Activity
import android.os.Bundle
import android.util.Log
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import mobilebridge.Mobilebridge
import org.json.JSONObject

class MainActivity : Activity() {
    private val tag = "VibeCodingAndroid"

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val density = resources.displayMetrics.density
        fun dp(value: Int): Int = (value * density).toInt()

        val input = EditText(this).apply {
            hint = "Enter text for Go core"
            setSingleLine(false)
            minLines = 2
            setText("hello")
        }
        val button = Button(this).apply {
            text = "Run Go Core"
        }
        val result = TextView(this).apply {
            text = "Result will appear here."
            textSize = 16f
        }

        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(24), dp(20), dp(24))
            addView(input, LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT
            ))
            addView(button, LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT
            ).apply { topMargin = dp(12) })
            addView(result, LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT
            ).apply { topMargin = dp(16) })
        }

        setContentView(ScrollView(this).apply { addView(content) })

        button.setOnClickListener {
            try {
                val service = Mobilebridge.newMobileService()
                val inputJson = JSONObject().put("text", input.text.toString()).toString()
                Log.d(tag, "RunJson input=$inputJson")
                val outputJson = service.runJson(inputJson)
                Log.d(tag, "RunJson output=$outputJson")
                val output = JSONObject(outputJson).optString("output", outputJson)
                result.text = output
            } catch (t: Throwable) {
                Log.e(tag, "Go bridge call failed", t)
                result.text = "Error: ${t.message ?: t::class.java.simpleName}"
            }
        }
    }
}
